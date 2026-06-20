# Gopher Email
A CLI tool that uses the Gmail API to back up emails as `.eml` files, organized by date, with metadata stored in a SQLite database. Designed for reliability and idempotency, it runs in a docker container that is scheduled by Ofelia to run every 15 minutes, to incrementally download new emails from the gsave label.

Runs independently from lumin_gopher, but is designed to be used in conjunction. The lumin_gopher pipeline will periodically check the gopher-email database for new entries, and if found, will copy the corresponding .eml files into the lumin_gopher pipeline for processing.


## Implementation Complete

**Go 1.26.4** installed to go. Binary at gopher-email.

### Files created

| Path | Purpose |
|---|---|
| config.go | Viper-based YAML config loader |
| oauth.go | OAuth2 flow — loads/refreshes `token.json` (chmod 600), interactive first-run consent |
| client.go | Gmail API: list by label, fetch raw, batch modify — all calls wrapped with exponential backoff |
| db.go | `modernc.org/sqlite` (pure Go) — schema, `Exists`, `Insert`, `Delete`, `AllStoragePaths` |
| writer.go | Writes `.eml` to `storage/YYYY/MM/DD/<uuid>.eml`, returns path + SHA-256 |
| pipeline.go | 7-step atomic pipeline per message; leaves `gSave` on any failure for retry |
| repair.go | Walks filesystem for orphaned `.eml` files and re-indexes them into SQLite |
| main.go | Cobra CLI — `run` and `sync` subcommands, `--config` / `--verbose` flags |
| config.yaml | Default config template |
| Makefile | `build`, `test`, `lint`, `install`, `run`, `sync` targets |
| .gitignore | Excludes `credentials.json`, `token.json`, `*.db`, `storage/` |

### Next step

Drop `credentials.json` (from Google Cloud Console → Gmail API → OAuth2 Desktop App) into the project root, then run:
```bash
./bin/gopher-email run --config ./config.yaml --verbose
```
The OAuth URL will be printed to the terminal on first run.
