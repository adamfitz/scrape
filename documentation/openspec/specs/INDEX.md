# scrape Specification Index

## Purpose

scrape is a CLI tool for looking up manga (and eventually manhwa, light novels, and other media) by name via the MangaDex API, with a local SQLite database for caching results and tracking reading progress. It replaces the previous chapter-downloading scraper with a metadata lookup and collection management tool.

## Architecture

```
CLI Commands (cobra)
  |
  +--> Pipeline (shared library)
  |      |
  |      +--> Normalise: NFKC + supplemental fold + custom layers
  |      +--> Data: local DB + MangaDex API
  |      +--> All data ingestion flows through pipeline
  |
  +--> Name Normalization (shared library)
  |      |
  |      +--> Unicode folding: confusable chars → ASCII (storage boundary)
  |      +--> Normalize raw titles to canonical forms (comparison boundary)
  |
  +--> MangaDex API Client (with rate limiting)
  |      |
  |      +--> Search manga by title
  |      +--> Extract multi-language titles (ja, ja-ro, ko, zh, en)
  |
  +--> SQLite Database Layer
         |
         +--> Local lookup before API calls
         +--> Store successful results (Unicode-folded at storage)
         +--> Backup and maintenance
```

## Capabilities

| # | Capability | Spec | Status |
|---|-----------|------|--------|
| 1 | [Name Normalization](name-normalization/spec.md) | Transform raw titles into deterministic canonical forms | Planned |
| 2 | [MangaDex API Client](mangadex-api-client/spec.md) | Query MangaDex API for manga metadata | Planned |
| 3 | [Rate Limiting](ratelimiting/spec.md) | Enforce MangaDex acceptable use policy rate limits | Planned |
| 4 | [SQLite Database](sqlite-database/spec.md) | Persistent storage for manga metadata and media tracking | Planned |
| 5 | [Single Lookup](single-lookup/spec.md) | Look up a single manga title via CLI | Planned |
| 6 | [Batch Lookup](batch-lookup/spec.md) | Look up multiple titles from text/CSV files | Planned |
| 7 | [Database Backup](database-backup/spec.md) | Compress and copy the database to a user-specified destination | Planned |
| 8 | [Database Maintenance](database-maintenance/spec.md) | SQLite VACUUM, integrity checks, and title normalization | Planned |
| 9 | [Shared Library](shared-library/spec.md) | Reusable packages: normalize, ratelimit, config, database | Planned |
| 10 | [CLI Commands](cli-commands/spec.md) | Cobra command structure and flag definitions | Planned |
| 11 | [Fuzzy Matching](fuzzy-matching/spec.md) | Sorensen-Dice similarity for approximate title matching | **Removed** — normalisation handles systematic variants; API handles typos |

## Shared Library Policy

All reusable functions MUST be placed in shared packages under `internal/`. No package may duplicate logic that exists in a shared package. Each shared package MUST be reviewed for applicability before new utility code is written. This policy is enforced by code review and documented in each shared package's godoc.

### Shared Packages

| Package | Location | Purpose |
|---------|----------|---------|
| `normalize` | `internal/normalize/` | NFKC + supplemental fold + Unicode case fold + custom layers |
| `pipeline` | `internal/pipeline/` | Composes normalise + data: Lookup, LookupAPI, BatchLookupStream, Ingest |
| `ratelimit` | `internal/ratelimit/` | Token bucket rate limiter for API calls |
| `config` | `internal/config/` | Config directory, paths, app settings |
| `database` | `internal/database/` | SQLite connection, migrations, schema (media-agnostic) |

## Database Schema

All media tables share a consistent column naming convention:

- `id` — integer primary key (autoincrement)
- `title` — primary display name
- `alt_title` — alternative/translated names (JSON text)
- `author` — author/creator name
- `description` — synopsis/description
- `cover_url` — cover image URL
- `url` — web URL for the media
- `source_id` — external source identifier (e.g., MangaDex ID)
- `status` — publication/status string
- `created_at` — record creation timestamp
- `updated_at` — record last update timestamp

### Tables

| Table | Columns |
|-------|---------|
| `manga` | id, title, alt_title, author, description, cover_url, url, source_id, status, created_at, updated_at |
| `observation` | id, media_id, media_type, progress, title, url, status, created_at |
| `bookmarks` | id, media_id, media_type, chapter_id, note, created_at |
| `anime` | id, title, alt_title, author, description, cover_url, url, source_id, status, created_at, updated_at |
| `lightnovel` | id, title, alt_title, author, description, cover_url, url, source_id, status, created_at, updated_at |
| `webnovel` | id, title, alt_title, author, description, cover_url, url, source_id, status, created_at, updated_at |
| `webtoons` | id, title, alt_title, author, description, cover_url, url, source_id, status, created_at, updated_at |
| `title_index` | media_type, media_id (FK), normalised, original_title |

### Title Index

The `title_index` table provides O(log N) lookups by pre-computing the normalised form of every title variant at ingest time. Each media record produces one index entry per title variant (title + all alt titles). The pipeline maintains the index automatically — callers never touch it directly.

- Indexed columns: `(media_type, normalised)` for lookups, `(media_id)` for cascade deletes
- Foreign key: `media_id` → media table `id` with `ON DELETE CASCADE`
- Rebuilt by `NormalizeAllTitles` and automatically on pipeline startup for existing databases

## Acceptable Use Policy Compliance

The MangaDex API requires:
- A `User-Agent` header that is not spoofed
- No `Via` header (no non-transparent proxies)
- Rate limit of ~5 requests/second per IP
- Credit MangaDex and scanlation groups in any application
- No ads or paid services wrapping the API

## Cross-Platform Requirements

- Pure Go with no CGO dependencies
- SQLite via `modernc.org/sqlite` (pure Go driver)
- Config directory via `os.UserConfigDir()` (XDG on Linux, AppData on Windows)
- Path construction via `filepath.Join` (never hardcoded separators)

## Media Type Extensibility

The schema is designed to accommodate future media types. New tables follow the same column naming convention. The `observation` and `bookmarks` tables use `media_type` string columns to reference any media table, avoiding foreign key coupling to a specific table.
