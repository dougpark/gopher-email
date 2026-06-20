Design Specification: Gopher Email Service (GES)
1. Overview
The Gopher Email Service (GES) is a Go-based CLI utility designed to ingest emails from a designated Gmail folder, preserve them as immutable .eml blobs, and index them into a local SQLite database. This creates a platform-agnostic data store where downstream services (like Lumin) can interact with emails as generic data objects rather than proprietary email formats.
2. System Architecture
The system operates as an event-driven worker. It connects to the Gmail API, processes the queue, and manages state across two storage layers.
2.1 Component Mapping
• Gmail Provider: The source of truth for incoming "to-process" items.
• GES CLI (Go): The engine that handles OAuth2, API communication, MIME parsing, and persistence.
• Local File System: Stores raw .eml blobs.
• SQLite Database: Stores metadata, indexing, and pointers to the file system.
3. Data Schema
3.1 SQLite Schema (email_archive.db)
To ensure downstream services don't need to know the source, we treat the email as a generic Data Resource.
CREATE TABLE email_archive_items (
    id TEXT PRIMARY KEY,        -- Unique hash or original Gmail ID
    source_type TEXT,           -- "email"
    created_at DATETIME,        -- Date of the email
    processed_at DATETIME,      -- Date of ingestion
    sender TEXT,
    subject TEXT,
    storage_path TEXT,          -- Relative path on local filesystem
    metadata JSON               -- Full headers (From, To, Cc, etc.)
);

4. Operational Logic
The utility follows an "Atomic Ingestion" pattern:
1. Authorization: Utilize google.golang.org/api/gmail/v1. Authenticate via token.json (OAuth2).
2. Fetch Queue: Query messages with label:gSave.
3. Extraction: • Retrieve message in raw format. • Decode Base64 payload.
4. Persistence: • Write content to storage/YYYY/MM/DD/<UUID>.eml. • Calculate a checksum for integrity.
5. Database Sync: Insert record into SQLite with the local filesystem path.
6. Cleanup (Atomicity): • Verify file existence and SQLite commit. • BatchModify: Remove gSave label, apply gArchive label. • If any step fails, the message remains in gSave for a retry.
5. CLI Interface Design
The tool will be invoked as a cron job or manual command.
# Basic Usage
gopher-email run --config ./config.yaml

# Verbose Logging for Debugging
gopher-email run --verbose

# Repair Mode: Re-index missing files from filesystem to DB
gopher-email sync --path ./storage

6. Implementation Guidelines
6.1 MIME Parsing
Do not rely on regex. Use the github.com/emersion/go-message/mail library to extract headers and body parts. This ensures that even complex nested multipart emails are parsed correctly.
6.2 Error Handling & Resilience
• Idempotency: Before processing a Gmail ID, check the SQLite database. If the ID exists, skip it to prevent duplicates.
• Graceful Degradation: If the API request fails, use an exponential backoff strategy for retries.
6.3 Decoupling Strategy
Downstream applications like Lumin must interact only with the SQLite database. To request the content, the application reads the storage_path column and serves the file. The database abstraction layer prevents Lumin from ever needing a Google Cloud API key or IMAP credentials.
7. Security Considerations
• Credential Storage: token.json should be stored with restricted Unix permissions (chmod 600).
• PII Sensitivity: Ensure the SQLite database is located on an encrypted partition, as it will contain the full text of your personal emails.
• Scoped Access: Use the gmail.gmail_modify scope rather than gmail.gmail_full, adhering to the principle of least privilege.