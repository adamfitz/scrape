# shared-library Specification

## Purpose

Define the shared library packages and the policy for code reuse across the scrape codebase.

## Shared Packages

### `internal/normalize/`

**Purpose:** Title normalization — NFKC + supplemental fold, custom separators/noise/stop words, Unicode case fold.

**Key exports:**
- `New(config NormalizationConfig) *Normalizer`
- `(*Normalizer).Normalize(name string) (string, error)` — full canonical form (NFKC → supplemental → custom → case fold)
- `(*Normalizer).MustNormalize(name string) string` — full canonical form (panics on error)
- `FoldUnicode(s string) string` — NFKC + supplemental fold (preserves case/words, for storage)
- `NormalizeAltTitlesJSON(raw string) string` — full normalize all values in alt_title JSON (for comparison)
- `NormalizeAllTitlesJSON(raw string) string` — NFKC + supplemental fold all values in alt_title JSON (for storage)

**Usage:**
- `FoldUnicode` / `NormalizeAllTitlesJSON` applied at **storage boundaries** (pipeline Ingest)
- `Normalize` / `MustNormalize` applied at **comparison boundaries** (pipeline Lookup)
- `NormalizeAltTitlesJSON` applied at **comparison boundaries** (alt-title matching)

**Default separators:** `.`, `_`, `-`, `:`, `;`, `,`, `!`, `?`, `'`, `'`, `"`, `~`, `～`, `〜`

### `internal/pipeline/`

**Purpose:** Composes normalise + data into a single data boundary. All data ingestion flows through this component. Owns `mangadex/`, `ratelimit/`, and `normalise/` dependencies — commands never import these directly.

**Key exports:**
- `New(db *database.DB, opts Options) *Pipeline` — create pipeline at application startup
- `(*Pipeline).DB() *database.DB` — access underlying DB handle
- `(*Pipeline).Normalizer() *normalize.Normalizer` — access normaliser instance
- `(*Pipeline).Lookup(mediaType, title) (*database.Media, error)` — local DB-only lookup
- `(*Pipeline).LookupAPI(mediaType, query, opts) (*LookupResult, error)` — full flow: local DB → MangaDex API → validate → ingest
- `(*Pipeline).BatchLookup(mediaType, titles) (found, missing)` — local batch DB lookup
- `(*Pipeline).BatchLookupStream(mediaType, titles, opts) <-chan LookupResult` — batch with channel streaming
- `(*Pipeline).Ingest(m) (int64, error)` — normalise title/alt titles → upsert
- `(*Pipeline).IngestCandidate(mediaType, candidate) (*database.Media, error)` — ingest user-selected API candidate
- `(*Pipeline).NormalizeAllTitles() (int64, error)` — re-normalise all rows across all media types
- `QueryForAPI(title) string` — light pre-processing for MangaDex query (package-level)
- `Deduplicate(titles) []string` — dedup using normalised forms (package-level)
- `TitleOrFallback(altTitleJSON, lang, fallback) string` — extract title by language (package-level)
- `ParseAltTitles(raw) AltTitleData` — parse stored alt_title JSON (package-level)

**Types:**
- `Pipeline` — holds normaliser + DB handle; all data methods are on this struct
- `Options` — pipeline configuration (e.g. `RateLimit float64`)
- `PipelineError` — structured error with `Kind ErrorKind`, `Message string`, `Err error`
- `ErrorKind` — error category enum: `ErrNotFound`, `ErrAPIMatchRejected`, `ErrAPIFailed`, `ErrIngestFailed`, `ErrDatabase`
- `LookupService` — interface for testing (`Lookup`, `LookupAPI`, `BatchLookup`, `Ingest`, `NormalizeAllTitles`)
- `AltTitleData` — structured multi-language title data
- `Candidate` — API match candidate for disambiguation
- `LookupResult` — result with Source, Media, Error, Candidates
- `LookupOptions` — LocalOnly, Limit
- `BatchLookupOptions` — LocalOnly, Limit
- `Source` — SourceDB or SourceAPI

**Usage:**
- Create one `*Pipeline` at application startup via `New(db, opts)` and reuse for the process lifetime
- Commands import only `pipeline/` and `database/` — never call normaliser, mangadex, or ratelimit functions directly
- Pipeline is the single entry point for all data operations
- Pipeline streams batch results via channels — callers handle progress display

**Execution model:**
- Each pipeline call is synchronous and self-contained — it returns a result when done
- Pipeline methods do not spawn background goroutines (except `BatchLookupStream` which streams via a channel)
- Pipeline does not manage concurrency — the calling application is responsible for goroutines, mutexes, and lifecycle management if needed
- Pipeline methods do not modify shared mutable state — they read from DB, normalise in local variables, write back through the DB handle

### `internal/ratelimit/`

**Purpose:** Token bucket rate limiter for API call throttling.

**Key exports:**
- `New(requestsPerSecond float64) *RateLimiter`
- `(*RateLimiter).Wait(ctx context.Context) error`
- `(*RateLimiter).Allow() bool`
- `(*RateLimiter).Stop()`

**Default rate:** 4 req/s (80% of MangaDex's 5 req/s limit)

### `internal/config/`

**Purpose:** Application configuration, paths, and settings.

**Key exports:**
- `ConfigDir() (string, error)` — returns `~/.config/scrape/`
- `DBPath() (string, error)` — returns full path to `scrape.db`
- `LogPath() (string, error)` — returns full path to `scrape.log`
- `EnsureConfigDir() error` — creates config dir if missing

### `internal/database/`

**Purpose:** SQLite connection, schema creation, queries, and title index. Media-agnostic — no normalisation logic.

**Key exports:**
- `Open(path string) (*DB, error)`
- `(*DB).Close() error`
- `(*DB).EnsureSchema() error`
- `(*DB).GetMediaBySourceID(mediaType, sourceID string) (*Media, error)` — lookup by source ID
- `(*DB).GetMediaByID(mediaType, id int64) (*Media, error)` — lookup by primary key
- `(*DB).AllMedia(mediaType string) ([]Media, error)` — fetch all rows for a media type
- `(*DB).AllMediaTypes() []string` — returns all media type table names
- `(*DB).InsertMedia(m *Media) (int64, error)` — insert new record
- `(*DB).UpdateMedia(m *Media) error` — update existing record by ID
- `(*DB).UpsertMedia(m *Media) (int64, error)` — insert or update by (type, source_id)
- `(*DB).Backup(dest string) error` — VACUUM INTO
- `(*DB).Vacuum() error`
- `(*DB).IntegrityCheck() (string, error)`
- `(*DB).Stats() (map[string]int, error)`

**Title Index methods:**
- `(*DB).IndexTitle(mediaType, mediaID, normalised, original) error` — add single entry
- `(*DB).IndexTitles(mediaType, mediaID, titles map[string]string) error` — replace all entries for a record (transactional)
- `(*DB).ClearTitleIndex(mediaType, mediaID) error` — remove entries for a record
- `(*DB).RebuildTitleIndex(mediaType) error` — clear all entries for a media type
- `(*DB).QueryTitleIndex(mediaType, normalised) (int64, error)` — O(log N) lookup by normalised title
- `(*DB).BatchQueryTitleIndex(mediaType, normalised []string) (map[string]int64, error)` — batch lookup
- `(*DB).TitleIndexSize(mediaType) (int, error)` — count entries for a media type
- `(*DB).TitleIndexSizeTotal() (int, error)` — count all entries

### `internal/mangadex/`

**Purpose:** MangaDex API client with rate limiting.

**Key exports:**
- `New(rl *ratelimit.RateLimiter) *Client`
- `(*Client).SearchManga(title string, limit int) ([]MangaResult, error)`
- `ExtractTitles(m MangaResult) map[string]string`
- `ExtractAuthorName(relationships []Relationship) string`
- `ExtractCoverURL(mangaID string, relationships []Relationship) string`

## Architecture Rule

All packages under `internal/` are importable only within the module. No circular dependencies.

```
commands/ --> internal/pipeline/
           --> internal/database/
           --> internal/config/

internal/pipeline/ --> internal/normalize/
                   --> internal/database/
                   --> internal/mangadex/
                   --> internal/ratelimit/

internal/mangadex/ --> internal/ratelimit/
internal/database/ --> internal/config/
```

## Policy

Before writing new utility code, check existing shared packages for applicable functions. No package may duplicate logic from a shared package.
