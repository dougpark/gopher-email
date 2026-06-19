Read [](file:///home/dougpark/Production/gopher-email/DESIGN.md)

Now let me research the Go dev environment on this Ubuntu server and the relevant libraries in parallel.  

Research Go env setup

Research Go Gmail and SQLite libs



Read [](file:///home/dougpark/Production/gopher-email)

Searched for regex `go\.mod|go\.sum|dockerfile|ubuntu`, no results

Fetched 2 resources

Searched googleapis/google-api-go-client for "Gmail API v1 authenticate OAuth2 fetch messages labels batch modify", 64 results

Searched emersion/go-message for "parse eml mime mail headers From To Subject Date Cc body", 64 results

Searched mattn/go-sqlite3 for "SQLite database connection usage", 64 results

Searched modernc-org/sqlite for "pure Go SQLite driver no cgo", 64 results

Searched spf13/cobra for "CLI subcommands flags tutorial", 64 results

Searched spf13/viper for "YAML config parsing CLI", 64 results

Searched google/uuid for "UUID generation v4", 64 results

Searched cenkalti/backoff for "exponential backoff retry", 64 results







Fetched https://go.dev/doc/install

Fetched https://golang.org/doc/effective_go

Fetched https://github.com/golang-standards/project-layout







Fetched https://github.com/mattn/go-sqlite3

Fetched https://golang.org/pkg/testing/

Fetched https://github.com/golangci/golangci-lint







Fetched https://pkg.go.dev/github.com/stretchr/testify

Fetched https://developers.google.com/identity/protocols/oauth2

Fetched https://github.com/spf13/cobra

Now I have enough to build a comprehensive plan. Let me save it and present it. 

Created memory file [](file:///home/dougpark/.vscode-server/data/User/workspaceStorage/9e97c685d4cf6d2b4977501f50817d4b/GitHub.copilot-chat/memory-tool/memories/MDdiZTA3YTctMWY0Yy00ZGJhLWJlOTEtYzliYWI0MTNhZmVh/plan.md)

Here is the full implementation plan:

---

## Plan: Gopher Email Service (GES) — Go CLI Implementation

**TL;DR:** Build `gopher-email` as a Go CLI using Cobra (subcommands), Viper (config), `modernc.org/sqlite` (pure Go, no cgo), the official Gmail API client, and `go-message` for MIME parsing. Follows an atomic ingestion pipeline: fetch → parse → write `.eml` → SQLite insert → label swap.

---

### Phase 0: Environment Setup

1. **Verify/install Go** — Check `go version`; if missing or outdated, download from go.dev/dl, install to `/usr/local/go`, add to PATH in `~/.bashrc`.
2. **No C toolchain needed** — `modernc.org/sqlite` is pure Go, so no `build-essential` or `libsqlite3-dev` required.
3. **Google Cloud credentials** — Enable Gmail API in Google Cloud Console, create an OAuth 2.0 Desktop App credential, download `credentials.json` to project root (gitignored). `token.json` is created on first `run`.
4. **Initialize Go module** — `go mod init github.com/dougpark/gopher-email` in the project directory.

---

### Phase 1: Project Scaffolding

5. **Directory layout**:
   ```
   gopher-email/
   ├── cmd/gopher-email/main.go       # Cobra root command entry point
   ├── internal/
   │   ├── config/config.go           # Viper config loader + Config struct
   │   ├── auth/oauth.go              # OAuth2 token.json flow
   │   ├── gmail/client.go            # Gmail API wrapper (list, fetch raw, batch modify)
   │   ├── storage/writer.go          # .eml file writer (storage/YYYY/MM/DD/<uuid>.eml)
   │   ├── db/db.go                   # SQLite schema, Exists(), Insert()
   │   ├── ingestion/pipeline.go      # Atomic ingestion orchestrator
   │   └── sync/repair.go             # sync --path re-indexer
   ├── config.yaml
   ├── go.mod / go.sum
   ├── Makefile
   └── .gitignore
   ```

6. **Dependencies** (`go get`):
   - `github.com/spf13/cobra` — CLI subcommands/flags
   - `github.com/spf13/viper` — YAML config + env var overrides
   - `google.golang.org/api/gmail/v1` + `golang.org/x/oauth2/google` — Gmail API + OAuth2
   - `github.com/emersion/go-message/mail` — RFC 5322 MIME parsing
   - `modernc.org/sqlite` — pure-Go SQLite driver
   - `github.com/google/uuid` — UUID v4 filenames
   - `github.com/cenkalti/backoff/v4` — exponential backoff for API retries

---

### Phase 2: Core Packages *(steps 7–12 are independent, can be built in parallel)*

7. **`internal/config`** — Viper loads `config.yaml`; struct fields: `StorageRoot`, `DBPath`, `CredentialsFile`, `TokenFile`, `InboundLabel`, `ArchiveLabel`.
8. **`internal/auth`** — Reads `credentials.json`, loads/refreshes `token.json`. First run: prints auth URL, reads code from stdin, writes token with `os.OpenFile(..., 0600)`.
9. **`internal/gmail`** — `ListByLabel(label)`, `GetRaw(msgID)`, `BatchModify(ids, remove, add)`. All API calls wrapped in `backoff.Retry` with exponential backoff.
10. **`internal/db`** — Opens DB via `modernc.org/sqlite`, applies `CREATE TABLE IF NOT EXISTS email_archive_items` schema from DESIGN.md. Methods: `Exists(id) bool`, `Insert(item) error`.
11. **`internal/storage`** — Accepts raw bytes + timestamp. Builds `storage/YYYY/MM/DD/<uuid>.eml` path, writes file, returns path + SHA-256 checksum (stored in `metadata` JSON column).
12. **`internal/ingestion`** — Atomic pipeline per message ID:
    1. `db.Exists(id)` → skip if true (idempotency)
    2. `gmail.GetRaw(id)` → base64 decode
    3. Parse headers via `go-message/mail` (sender, subject, date, To, Cc)
    4. `storage.Write(raw, date)` → path + checksum
    5. `db.Insert(item)`
    6. Verify file on disk → `gmail.BatchModify(id, remove=[gSave], add=[gArchive])`
    7. Any failure before step 6: log + leave in `gSave` for retry

---

### Phase 3: CLI Commands *(depends on Phase 2)*

13. **`cmd/gopher-email/main.go`** — Cobra root with `--config` and `--verbose` persistent flags.
14. **`run` subcommand** — Calls `gmail.ListByLabel("gSave")`, iterates IDs, calls `ingestion.Process(id)` for each.
15. **`sync` subcommand** — Walks `--path` filesystem for `*.eml` files not in DB, re-parses headers, inserts missing records.

---

### Phase 4: Config, Security & Build

16. **`config.yaml` template** with defaults for all paths and label names.
17. **`.gitignore`** — Excludes `credentials.json`, `token.json`, `*.db`, `storage/`.
18. **Makefile** — `build`, `test`, `lint`, `install` targets.
19. **Cron job** — Example entry running every 15 min, logging to `/var/log/gopher-email.log`.

---

### Verification

1. `go build Production.` compiles cleanly
2. First `gopher-email run --verbose`: OAuth flow completes, `token.json` created with `chmod 600`
3. After processing: `.eml` files present in `storage/`, SQLite row exists (`sqlite3 email_archive.db "SELECT * FROM email_archive_items LIMIT 5;"`)
4. Gmail: processed messages have `gArchive`, no longer have `gSave`
5. Re-run: already-processed IDs skipped (idempotency)
6. `gopher-email sync --path ./storage` re-indexes without duplicates
7. `go test Production.` passes

---

### Decisions

- **`modernc.org/sqlite`** over `mattn/go-sqlite3` — pure Go, single static binary, no cgo
- **`viper`** over bare `yaml.v3` — native Cobra integration, env var override support
- **UUID v4** for filenames — random, no time metadata leakage
- **`gmail.modify` scope** only — least privilege per DESIGN.md §7
- **Out of scope**: attachment extraction, full-text body search, web UI, multi-account

### Further Considerations

1. **Headless OAuth first-run**: The token exchange requires stdin input. On this Ubuntu server, pre-generate `token.json` on a desktop machine and `scp` it over, then apply `chmod 600`. Worth documenting in the README.
2. **`metadata` JSON**: All RFC 5322 headers will be stored as `map[string][]string` marshalled to JSON — covers all headers without future schema migrations.