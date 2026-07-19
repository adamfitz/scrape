# scrape Transformation Plan

## Overview

Convert scrape from a manga chapter downloader into a metadata lookup and collection management tool for manga (and eventually other media). The tool queries MangaDex by title, stores results in a local SQLite database, and supports batch operations with local-first lookup strategy.

## Repository Structure (Target)

```
scrape/
├── scrape.go                          # Main entry point (updated)
├── go.mod                             # Updated dependencies
├── go.sum
├── openspec/
│   └── specs/                         # OpenSpec specifications
│       ├── INDEX.md
│       ├── name-normalization/spec.md
│       ├── mangadex-api-client/spec.md
│       ├── ratelimiting/spec.md
│       ├── sqlite-database/spec.md
│       ├── single-lookup/spec.md
│       ├── batch-lookup/spec.md
│       ├── database-backup/spec.md
│       ├── database-maintenance/spec.md
│       ├── shared-library/spec.md
│       └── cli-commands/spec.md
├── commands/
│   ├── root.go                        # Root command (cleaned up)
│   ├── lookup.go                      # New: single lookup command
│   ├── batch.go                       # New: batch lookup command
│   ├── backup.go                      # New: backup command
│   ├── maintenance.go                 # New: maintenance subcommands
│   ├── khinsider.go                   # Refactored from sites.go
│   └── version.go                     # Existing (unchanged)
├── internal/
│   ├── normalize/
│   │   ├── normalize.go               # Ported from resolve
│   │   └── normalize_test.go          # Ported from resolve
│   ├── ratelimit/
│   │   └── ratelimit.go               # New: token bucket rate limiter
│   ├── config/
│   │   └── config.go                  # New: paths and settings
│   ├── database/
│   │   ├── database.go                # New: SQLite connection + schema
│   │   ├── queries.go                 # New: all query methods
│   │   └── database_test.go           # New: database tests
│   └── mangadex/
│       ├── client.go                  # Ported+extended from resolve
│       └── client_test.go             # New: client tests
├── khinsider/
│   └── khinsider.go                   # Existing (unchanged)
├── version/
│   └── version.go                     # Existing (unchanged)
├── .github/workflows/release-on-merge.yml  # Existing (unchanged)
└── .goreleaser.yml                    # Existing (unchanged)
```

## Implementation Phases

### Phase 1: Remove Old Packages and Clean CLI

**Goal:** Remove all manga scraper packages and clean up the CLI structure.

**Files to delete:**
```
asura/asura.go
cfotz/cfotz.go
hls/hls.go
iluim/iluim.go
kunmanga/kunmanga.go
manhuaus/manhuaus.go
mgeko/mgeko.go
orv/orv.go
pmeig/pmuig.go
ravenscans/ravenscans.go
rizzfables/rizzfables.go
stonescape/stonescape.go
xbato/xbato.go
webClient/webClient.go
parser/parser.go
parser/parser_test.go
commands/sites.go
```

**Files to modify:**
- `commands/root.go` — Remove all AddCommand calls except khinsider and version
- `scrape.go` — Change log path from `/var/log/scrape/scrape.log` to use config dir

**Steps:**
1. Delete all 14 manga scraper package directories
2. Delete `webClient/` package (no longer needed — khinsider doesn't use it)
3. Delete `parser/` package (no longer needed — khinsider uses its own sanitize)
4. Delete `commands/sites.go` (contains all site commands)
5. Create `commands/khinsider.go` — extract khinsider command from sites.go
6. Update `commands/root.go` — only register khinsiderCmd and versionNumber
7. Run `go mod tidy` to remove unused dependencies
8. Verify build compiles

**Verification:**
- `go build ./...` succeeds
- `go vet ./...` passes
- `scrape version` works
- `scrape khinsider --help` works

---

### Phase 2: Create Shared Libraries

**Goal:** Build the foundational shared packages that all other code depends on.

#### 2a. `internal/config/config.go`

```go
package config

// Functions:
// ConfigDir() (string, error)       — returns ~/.config/scrape/
// DBPath() (string, error)          — returns ~/.config/scrape/scrape.db
// LogPath() (string, error)         — returns ~/.config/scrape/scrape.log
// EnsureConfigDir() error           — creates config dir if missing
```

- Use `os.UserConfigDir()` for cross-platform
- Append `scrape` subdirectory
- Create with `0755` permissions if missing

#### 2b. `internal/normalize/normalize.go`

Port from `/home/adam/projects/resolve/internal/normalize/normalize.go`:
- Copy `normalize.go` and `normalize_test.go`
- Replace `domain.NormalizationConfig` with local struct
- Replace `domain.CanonicalForm` with `string`
- Update module imports
- Run tests to verify

#### 2c. `internal/ratelimit/ratelimit.go`

```go
package ratelimit

// RateLimiter uses a token bucket algorithm.
type RateLimiter struct {
    ticker   *time.Ticker
    tokens   chan struct{}
    done     chan struct{}
}

// New creates a rate limiter. requestsPerSecond controls the fill rate.
// Burst is always 1 (one request at a time, evenly spaced).
func New(requestsPerSecond float64) *RateLimiter

// Wait blocks until a token is available or ctx is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error

// Allow returns true if a token is available without blocking.
func (r *RateLimiter) Allow() bool

// Stop releases the rate limiter's goroutine.
func (r *RateLimiter) Stop()
```

- Internal goroutine refills tokens via time.Ticker
- `Wait()` selects on token channel or context.Done
- Default: 4 req/s (80% of MangaDex's 5 req/s limit)

#### 2d. `internal/database/database.go`

```go
package database

import "database/sql"

type DB struct {
    conn *sql.DB
    path string
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error)

// Close closes the database connection.
func (db *DB) Close() error

// EnsureSchema creates all tables if they don't exist.
func (db *DB) EnsureSchema() error
```

Schema creation SQL:
- manga, observation, bookmarks, anime, lightnovel, webnovel, webtoons
- All as specified in sqlite-database/spec.md
- Enable WAL mode: `PRAGMA journal_mode=WAL`
- Enable foreign keys: `PRAGMA foreign_keys=ON`

#### 2e. `internal/database/queries.go`

All query methods:
- `GetMangaByTitle(title string) (*Manga, error)` — case-insensitive LIKE
- `GetMangaByMangadexID(id string) (*Manga, error)` — exact match
- `BatchCheckTitles(titles []string) (found map[string]*Manga, missing []string)`
- `InsertManga(m *Manga) error`
- `UpdateManga(m *Manga) error`
- `Backup(dest string) error` — VACUUM INTO
- `Vacuum() error`
- `IntegrityCheck() (string, error)`
- `Stats() (map[string]int, error)`

**Verification:**
- `go test ./internal/...` passes
- `go vet ./internal/...` passes

---

### Phase 3: MangaDex API Client

**Goal:** Build a rate-limited MangaDex client with multi-language title extraction.

#### 3a. `internal/mangadex/client.go`

Port from `/home/adam/projects/resolve/internal/mangadex/client.go` with extensions:

```go
package mangadex

const baseURL = "https://api.mangadex.org"

type Client struct {
    http      *http.Client
    rateLimit *ratelimit.RateLimiter
    userAgent string
}

func New(rl *ratelimit.RateLimiter) *Client

// SearchManga queries MangaDex with rate limiting.
func (c *Client) SearchManga(title string, limit int) ([]MangaResult, error)

// ExtractTitles pulls all language variants from a manga result.
func ExtractTitles(m MangaResult) map[string]string

// ExtractAuthor gets the author name from includes.
func ExtractAuthor(m MangaResult) string

// MakeLookupResult builds a full LookupResult from a search result.
func MakeLookupResult(query string, m MangaResult) LookupResult
```

Key extensions over resolve's client:
- Rate limiter integration (every call goes through `rateLimiter.Wait()`)
- Extract ALL language variants (en, ja, ja-ro, ko, zh, zh-ro), not just English
- Add `?includes[]=author` and `?includes[]=cover_art` to requests
- Handle 429 responses with Retry-After header
- Configurable User-Agent

**Request construction:**
```
GET /manga?title={title}&limit={limit}&includes[]=author&includes[]=cover_art
```

**429 handling:**
```
if resp.StatusCode == 429 {
    retryAfter := resp.Header.Get("X-RateLimit-Retry-After")
    // parse retryAfter as int (seconds), default to 1
    time.Sleep(time.Duration(seconds) * time.Second)
    // retry once
}
```

**Verification:**
- Manual test: `scrape lookup "One Piece"` returns correct data
- Rate limiter: requests are spaced ≥250ms apart

---

### Phase 4: SQLite Database Layer (completed in Phase 2d/2e)

Already covered in Phase 2. Additional verification:
- `go test ./internal/database/...` — test schema creation, insert, query
- Test batch lookup performance with 1000 titles
- Test backup/restore cycle

---

### Phase 5: Lookup Commands

#### 5a. `commands/lookup.go`

```go
package commands

var lookupCmd = &cobra.Command{
    Use:   "lookup <title>",
    Short: "Look up a manga by name on MangaDex",
    Args:  cobra.ExactArgs(1),
    RunE:  runLookup,
}

func runLookup(cmd *cobra.Command, args []string) error {
    // 1. Normalize title
    // 2. Check local database
    // 3. If not found, query MangaDex (rate-limited)
    // 4. Store result in database
    // 5. Display result
}
```

Flags: `--json`, `--limit` (int, default 5)

#### 5b. `commands/batch.go`

```go
package commands

var batchCmd = &cobra.Command{
    Use:   "batch <file>",
    Short: "Look up multiple manga titles from a file",
    Args:  cobra.ExactArgs(1),
    RunE:  runBatch,
}

func runBatch(cmd *cobra.Command, args []string) error {
    // 1. Read file (detect .txt vs .csv)
    // 2. Parse titles (skip empty, skip # comments)
    // 3. Deduplicate titles
    // 4. Batch check local database
    // 5. Build "missing" list
    // 6. Query MangaDex for missing titles (rate-limited)
    // 7. Store new results
    // 8. Display progress and summary
}
```

Flags: `--json`, `--limit` (int, default 5)

**File parsing:**
- `.txt`: one title per line, skip empty lines, skip lines starting with `#`
- `.csv`: read first column (after header row), trim whitespace

**Verification:**
- `echo "One Piece\nNaruto" | scrape batch -` — stdin support (nice to have)
- `scrape batch titles.txt` — text file
- `scrape batch titles.csv` — CSV file
- Verify local-first: second run uses cache, no API calls

---

### Phase 6: Backup and Maintenance Commands

#### 6a. `commands/backup.go`

```go
var backupCmd = &cobra.Command{
    Use:   "backup <destination>",
    Short: "Backup the local database",
    Args:  cobra.ExactArgs(1),
    RunE:  runBackup,
}

func runBackup(cmd *cobra.Command, args []string) error {
    // 1. Open database
    // 2. If destination is a directory, generate timestamped filename
    // 3. VACUUM INTO destination
    // 4. Gzip compress
    // 5. Display result
}
```

#### 6b. `commands/maintenance.go`

```go
var maintenanceCmd = &cobra.Command{
    Use:   "maintenance",
    Short: "Database maintenance commands",
}

var vacuumCmd = &cobra.Command{
    Use:   "vacuum",
    Short: "Compact the database",
    RunE:  runVacuum,
}

var checkCmd = &cobra.Command{
    Use:   "check",
    Short: "Run integrity check",
    RunE:  runCheck,
}

var statsCmd = &cobra.Command{
    Use:   "stats",
    Short: "Display database statistics",
    RunE:  runStats,
}
```

**Verification:**
- `scrape backup ~/test-backup.db.gz` — creates compressed backup
- `scrape maintenance vacuum` — compacts database
- `scrape maintenance check` — returns "ok"
- `scrape maintenance stats` — shows table counts

---

### Phase 7: Final Integration

**Goal:** Update main entry point, clean up dependencies, verify everything works.

1. Update `scrape.go`:
   - Change log path to use `internal/config`
   - Handle missing config dir gracefully (don't fatal)
   - Add `--verbose` flag support

2. Update `commands/root.go`:
   - Register all new commands
   - Update help text

3. Update `go.mod`:
   - Add `modernc.org/sqlite` dependency
   - Remove unused dependencies via `go mod tidy`
   - Consider upgrading Go version if needed

4. Cross-platform verification:
   - `GOOS=linux GOARCH=amd64 go build` 
   - `GOOS=windows GOARCH=amd64 go build`
   - `GOOS=darwin GOARCH=amd64 go build`
   - `GOOS=darwin GOARCH=arm64 go build`

5. Run all tests:
   - `go test ./...`
   - `go vet ./...`

6. Update `.goreleaser.yml` if needed for new binary name/flags

## Dependency Changes

### Added
- `modernc.org/sqlite` — pure Go SQLite driver

### Removed (after `go mod tidy`)
- `github.com/PuerkitoBio/goquery` — only used by removed scrapers
- `github.com/chai2010/webp` — only used by removed scrapers
- `github.com/chromedp/cdproto` — only used by removed scrapers
- `github.com/chromedp/chromedp` — only used by removed scrapers
- `golang.org/x/image` — only used by removed scrapers

### Retained
- `github.com/gocolly/colly` — used by khinsider
- `github.com/spf13/cobra` — CLI framework

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| MangaDex rate limit exceeded | Token bucket at 4 req/s (80% of limit) |
| Database corruption | WAL mode, integrity checks, safe backup via VACUUM INTO |
| Cross-platform path issues | Always use `filepath.Join` and `os.UserConfigDir()` |
| Large batch files | Process in chunks, display progress, error resilience |
| MangaDex API changes | Structured response types with JSON tags, error wrapping |
