# Dashboard Stats

This milestone records durable stats for every `gopher-email run` invocation and
keeps a one-row snapshot for quick dashboard reads.

## Implemented Tables

### `run_stats`

One row per `run` command execution.

- `run_id` (TEXT, PK): unique run identifier
- `run_type` (TEXT): currently `run`
- `started_at` (DATETIME): run start time (UTC)
- `finished_at` (DATETIME): run finish time (UTC)
- `duration_ms` (INTEGER): run duration in milliseconds
- `status` (TEXT): `success`, `partial`, `failed`, `interrupted`
- `inbound_label` (TEXT): inbound Gmail label name
- `archive_label` (TEXT): archive Gmail label name
- `fetched_count` (INTEGER): message IDs fetched from Gmail query
- `processed_ok_count` (INTEGER): messages written + inserted successfully
- `skipped_exists_count` (INTEGER): skipped due to idempotency check
- `failed_count` (INTEGER): hard failures in pipeline
- `label_swap_error_count` (INTEGER): DB/file success but Gmail label swap failed
- `message` (TEXT): run summary/error text

### `system_status`

Single-row latest snapshot (`id = 1`) used by dashboards.

- `last_run` (DATETIME): when latest run finished
- `last_status` (TEXT): latest run status
- `emails_fetched` (INTEGER): latest run fetched count
- `emails_ingested` (INTEGER): latest run processed count
- `total_archived` (INTEGER): total rows in `email_archive_items`
- `message` (TEXT): latest run summary/error

## What "Current Ingest State" Means

Current ingest state is represented by `system_status`:

- Is ingestion healthy now: `last_status`
- Is pipeline moving data: `emails_fetched` vs `emails_ingested`
- Is backlog likely present: `emails_fetched > emails_ingested`
- Total archive size: `total_archived`
- Human-readable reason: `message`

## Useful Queries

Latest snapshot:

```sql
SELECT *
FROM system_status
WHERE id = 1;
```

Recent runs (newest first):

```sql
SELECT
	run_id,
	started_at,
	finished_at,
	duration_ms,
	status,
	fetched_count,
	processed_ok_count,
	skipped_exists_count,
	failed_count,
	label_swap_error_count,
	message
FROM run_stats
ORDER BY finished_at DESC
LIMIT 20;
```

