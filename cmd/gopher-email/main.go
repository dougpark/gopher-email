package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/dougpark/gopher-email/internal/auth"
	"github.com/dougpark/gopher-email/internal/config"
	"github.com/dougpark/gopher-email/internal/db"
	"github.com/dougpark/gopher-email/internal/gmail"
	"github.com/dougpark/gopher-email/internal/ingestion"
	"github.com/dougpark/gopher-email/internal/syncer"
)

var (
	configFile string
	verbose    bool
)

func main() {
	root := &cobra.Command{
		Use:   "gopher-email",
		Short: "Ingest Gmail messages into a local .eml archive",
	}

	root.PersistentFlags().StringVar(&configFile, "config", "./config.yaml", "path to config file")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose logging")

	root.AddCommand(runCmd(), syncCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// runCmd defines the `gopher-email run` subcommand.
func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Process all messages labelled with the inbound label",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			cfg, err := config.Load(configFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			configureLogging(verbose)

			httpClient, err := auth.NewHTTPClient(ctx, cfg.CredentialsFile, cfg.TokenFile)
			if err != nil {
				return fmt.Errorf("authenticating: %w", err)
			}

			gmailClient, err := gmail.New(ctx, httpClient)
			if err != nil {
				return fmt.Errorf("creating gmail client: %w", err)
			}

			database, err := db.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			runStat := db.RunStat{
				RunID:        uuid.NewString(),
				RunType:      "run",
				StartedAt:    time.Now().UTC(),
				Status:       "failed",
				InboundLabel: cfg.InboundLabel,
				ArchiveLabel: cfg.ArchiveLabel,
				Message:      "run started",
			}
			defer func() {
				runStat.FinishedAt = time.Now().UTC()
				runStat.DurationMs = runStat.FinishedAt.Sub(runStat.StartedAt).Milliseconds()
				if runStat.Message == "" {
					runStat.Message = runStat.Status
				}

				if err := database.InsertRunStat(runStat); err != nil {
					log.Printf("warning: failed to insert run stats: %v", err)
					return
				}

				totalArchived, err := database.CountArchived()
				if err != nil {
					log.Printf("warning: failed to count archived rows: %v", err)
					totalArchived = 0
				}

				snapshot := db.SystemStatus{
					LastRun:        runStat.FinishedAt,
					LastStatus:     runStat.Status,
					EmailsFetched:  runStat.FetchedCount,
					EmailsIngested: runStat.ProcessedOKCount,
					TotalArchived:  totalArchived,
					Message:        runStat.Message,
				}
				if err := database.UpsertSystemStatus(snapshot); err != nil {
					log.Printf("warning: failed to update system status snapshot: %v", err)
				}
			}()

			pipeline := ingestion.New(database, gmailClient, cfg.StorageRoot, cfg.InboundLabel, cfg.ArchiveLabel, verbose)

			log.Printf("fetching messages with label %q...", cfg.InboundLabel)
			ids, err := gmailClient.ListByLabel(ctx, cfg.InboundLabel)
			if err != nil {
				runStat.Message = fmt.Sprintf("listing messages: %v", err)
				return fmt.Errorf("listing messages: %w", err)
			}
			log.Printf("found %d message(s) to process", len(ids))
			runStat.FetchedCount = len(ids)

			var interrupted bool
			for _, id := range ids {
				select {
				case <-ctx.Done():
					interrupted = true
					runStat.Status = "interrupted"
					runStat.Message = fmt.Sprintf("interrupted: processed %d/%d messages", len(ids)-len(ids[indexOf(ids, id):]), len(ids))
					log.Printf("interrupted: processed %d/%d messages", len(ids)-len(ids[indexOf(ids, id):]), len(ids))
					return nil
				default:
				}

				result, err := pipeline.Process(ctx, id)
				if err != nil {
					log.Printf("error processing %s: %v", id, err)
					runStat.FailedCount++
					continue
				}
				if result.SkippedExists {
					runStat.SkippedExistsCount++
				}
				if result.Processed {
					runStat.ProcessedOKCount++
				}
				if result.LabelSwapFailed {
					runStat.LabelSwapErrorCount++
				}
			}

			if interrupted {
				return nil
			}

			if runStat.FailedCount > 0 {
				runStat.Status = "partial"
				runStat.Message = fmt.Sprintf("%d message(s) failed to process; they remain in %q for retry", runStat.FailedCount, cfg.InboundLabel)
				return errors.New(runStat.Message)
			}

			if runStat.LabelSwapErrorCount > 0 {
				runStat.Status = "partial"
				runStat.Message = fmt.Sprintf("run complete with %d label swap warning(s)", runStat.LabelSwapErrorCount)
			} else {
				runStat.Status = "success"
				runStat.Message = "done"
			}
			log.Printf("done")
			return nil
		},
	}
}

// syncCmd defines the `gopher-email sync` subcommand.
func syncCmd() *cobra.Command {
	var storagePath string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Re-index .eml files from the filesystem into the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			cfg, err := config.Load(configFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			configureLogging(verbose)

			if storagePath == "" {
				storagePath = cfg.StorageRoot
			}

			database, err := db.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			return syncer.Run(ctx, database, syncer.RepairOptions{
				StoragePath: storagePath,
				DBPath:      cfg.DBPath,
				Verbose:     verbose,
			})
		},
	}

	cmd.Flags().StringVar(&storagePath, "path", "", "override storage path to walk (defaults to config storage_root)")
	return cmd
}

func configureLogging(verbose bool) {
	if verbose {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	} else {
		log.SetFlags(log.Ldate | log.Ltime)
	}
}

// indexOf returns the index of id in ids, or len(ids) if not found.
func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return len(ids)
}
