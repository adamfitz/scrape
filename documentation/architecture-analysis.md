# Architecture Analysis: Data Boundary Design

Analysis of the proposed three-pillar architecture and its impact on normalisation guarantees.

---

## Table of Contents

- [Current Architecture Problems](#current-architecture-problems)
- [Proposed Architecture](#proposed-architecture)
- [Data Flow Boundaries](#data-flow-boundaries)
- [Data Component Design](#data-component-design)
- [CLI Responsibilities](#cli-responsibilities)
- [Migration Path](#migration-path)
- [User-Facing Layer: CLI and GUI](#user-facing-layer-cli-and-gui)
- [Recommendation](#recommendation)
- [Package Dependency Rules](#package-dependency-rules)

---

## Current Architecture Problems

The data flow trace reveals the previous data entry points and how they are now handled:

| Entry Point | Data Origin | Normalisation Applied |
|-------------|------------|----------------------|
| CLI argument | User types title | Pipeline normalises at lookup/ingest |
| File import | `.txt` / `.csv` file | Pipeline normalises at dedup + batch lookup |
| MangaDex API | HTTP JSON response | Pipeline normalises at ingest (FoldUnicode) |
| Database read | SQLite query | Pipeline normalises for comparison |
| Database write | InsertMedia / UpdateMedia | None — raw data stored (normalisation is pipeline's job) |
| Maintenance | NormalizeAllTitles | Pipeline re-normalises all 5 tables |

### Media Types

The application stores seven table types, all sharing the same `(title, alt_title)` column pattern:

| Table | Status | Normalisation |
|-------|--------|--------------|
| `manga` | Active | Pipeline normalises at ingest + all titles |
| `anime` | Schema exists | Pipeline normalises at ingest + all titles |
| `lightnovel` | Schema exists | Pipeline normalises at ingest + all titles |
| `webnovel` | Schema exists | Pipeline normalises at ingest + all titles |
| `webtoons` | Schema exists | Pipeline normalises at ingest + all titles |
| `observation` | Schema exists | Tracking/progress, not title-based |
| `bookmarks` | Schema exists | User bookmarks, not title-based |

All title-bearing tables share identical column schemas: `title`, `alt_title` (JSON), `author`, `description`, `cover_url`, `url`, `source_id`, `status`, `created_at`, `updated_at`. The pipeline handles all of them uniformly via the `MediaType` parameter.

### The Core Problem (Now Resolved)

Normalisation was applied **inside** the database layer and **inside** the command layer, with no single point that guaranteed all data was normalised before it entered the system. This created:

1. ~~**Inconsistent application** — manga table gets FoldUnicode, all other title-bearing tables get nothing.~~ **Resolved** — pipeline normalises all 5 tables uniformly.
2. ~~**Scattered logic** — normalisation happens in 6 different locations across 4 files.~~ **Resolved** — all normalisation lives in `internal/pipeline/` and `internal/normalize/`.
3. ~~**No enforcement** — a new data path can easily bypass normalisation.~~ **Resolved** — pipeline is the only entry point for data operations.
4. ~~**No media abstraction** — `InsertManga`, `GetMangaByTitle`, etc. are manga-specific.~~ **Resolved** — `InsertMedia`, `AllMedia`, `UpsertMedia` are media-agnostic.
5. ~~**Two incompatible normalisation tiers** — FoldUnicode at write time, full normalise at read time.~~ **Resolved** — pipeline applies FoldUnicode at ingest, full normalise at comparison. Clear rule: FoldUnicode preserves readability, full normalise enables matching.

---

## Proposed Architecture: Three Pillars

```
┌─────────────────────────────────────────────────┐
│                   PIPELINE                       │
│  Defines the data boundary. All data flows       │
│  through here. Applies normalisation at every    │
│  entry and exit point.                           │
│                                                  │
│  ┌──────────────┐       ┌──────────────┐        │
│  │  Normalise    │       │    Data      │        │
│  │  Component    │       │  Component   │        │
│  └──────────────┘       └──────────────┘        │
│                                                  │
└─────────────────────────────────────────────────┘
```

### Pillar 1: Normalisation Component (`internal/normalise/`)

**Responsibility**: Transform text. Nothing else.

- Takes a string, returns a string.
- No database, no API, no file I/O.
- Pure functions, stateless (the normaliser is immutable after construction — safe for concurrent use).
- Individually testable with known inputs/outputs.
- Can be swapped or upgraded without touching other pillars.

**Current state**: Exists as `internal/normalize/`. NFKC + supplemental fold replaces the manual `unicodeFold` map. Unicode case folding replaces `strings.ToLower`. This pillar is in place.

### Pillar 2: Data Component (`internal/data/`)

**Responsibility**: Store and retrieve structured data. Nothing else.

- Knows about SQLite, file formats, API response shapes.
- Does **NOT** normalise. Raw data goes in, raw data comes out.
- Provides a **media-agnostic interface** — not `InsertManga` / `GetMangaByTitle`, but `InsertMedia` / `GetMediaByTitle` that works for all media types.
- Individually testable with mock databases or real SQLite.

#### Media-Agnostic Design

The data component defines a single `Media` type that represents any title-bearing record:

```go
type MediaType int

const (
    MediaTypeManga     MediaType = iota
    MediaTypeAnime
    MediaTypeLightNovel
    MediaTypeWebNovel
    MediaTypeWebtoon
)

type Media struct {
    ID          int64
    Type        MediaType
    Title       string
    AltTitle    string  // JSON
    Description string
    URL         string
    Status      string
    SourceID    string  // MangaDex ID, AniList ID, etc.
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

The data component exposes generic methods:

```go
// Storage
InsertMedia(m *Media) (int64, error)
UpdateMedia(m *Media) error
UpsertMedia(m *Media) (int64, error)  // insert or update by SourceID

// Lookup — returns raw, un-normalised data
GetMediaBySourceID(id string) (*Media, error)  // searches all tables
AllMedia(mediaType MediaType) ([]Media, error) // all rows for a type

// Maintenance
Backup(dest string) error
Vacuum() error
IntegrityCheck() (string, error)
Stats() (map[string]int, error)
```

Media-specific fields (volumes for light novels, cover URL for manga) are handled via struct embeddings. The base `Media` struct contains the shared fields. Media-specific structs embed `Media` and add their own fields:

```go
// Manga extends Media with manga-specific fields.
type Manga struct {
    Media
    Author   string
    CoverURL string
    MangaDexID string
}

// Anime extends Media with anime-specific fields.
type Anime struct {
    Media
    // anime-specific fields as needed
}
```

The title and alt_title columns are shared across all media types and are the only columns that need normalisation. Media-specific fields are only relevant when ingesting from specific APIs or displaying data — the pipeline operates on the shared `Media` fields.

**Current state**: Exists as `internal/database/`. Normalisation has been removed — `InsertMedia`, `UpdateMedia`, `UpsertMedia` store raw data without folding. All methods are media-agnostic via `MediaType` parameter. `AllMedia(mediaType)` returns `[]Media` for any table. `GetMediaBySourceID(id)` searches across all tables. The `Media` struct has shared fields (`Title`, `AltTitle`, `Author`, `Description`, `CoverURL`, `URL`, `SourceID`, `Status`). All 5 title-bearing tables share identical column schemas (`title`, `alt_title`, `author`, `description`, `cover_url`, `url`, `source_id`, `status`).

### Pillar 3: Pipeline Component (`internal/pipeline/`)

**Responsibility**: Define the data boundary. Be the **only** place where data enters or exits the system.

- Every external input (user, file, API) flows through the pipeline.
- The pipeline calls the normalisation component to transform data.
- The pipeline calls the data component to store/retrieve data.
- The data component never calls normalisation. The normalisation component never calls data.
- Handles **all media types** through a single, consistent interface.
- Individually testable by mocking both normalise and data.

#### Media-Agnostic Pipeline Interface

The pipeline exposes operations that work for any media type:

```go
// Lookup any media type by title (exact match via normalised comparison)
Lookup(db *database.DB, mediaType database.MediaType, title string) (*database.Media, error)

// Batch lookup — returns found entries keyed by original title, and missing titles
BatchLookup(db *database.DB, mediaType database.MediaType, titles []string) (map[string]*database.Media, []string)

// Ingest normalises and upserts a media record
Ingest(db *database.DB, m *database.Media) (int64, error)

// Maintenance — normalise all stored titles across all media tables
NormalizeAllTitles(db *database.DB) (int64, error)

// Utilities
QueryForAPI(title string) string           // normalise for external API search
Deduplicate(titles []string) []string      // dedup using normalised forms
TitleOrFallback(altTitleJSON, lang, fallback string) string  // extract title by language
NewNormalizer() *normalize.Normalizer      // for callers needing direct access
```

Each method:
1. Normalises the query/input via the normalisation component.
2. Calls the data component with normalised data.
3. Returns results.

The pipeline is the **only** package that imports both `normalise/` and `data/`. Commands only import `pipeline/`.

**Current state**: Exists as `internal/pipeline/`. Composes normalise + data via `Lookup` (DB-only), `LookupAPI` (full DB → MangaDex flow), `BatchLookup`, `BatchLookupStream` (channel-based), `Ingest`, `IngestCandidate`, `NormalizeAllTitles`, `QueryForAPI`, `Deduplicate`, `TitleOrFallback`, `NewNormalizer`, `ParseAltTitles`. The pipeline owns `mangadex/`, `ratelimit/`, and `normalise/` dependencies. Commands import only `pipeline/` and `database/` — no direct `mangadex.*`, `ratelimit.*`, or `normalize.*` calls from commands.

### Pipeline Execution Model

The pipeline is a **synchronous, single-call library**. Each function executes a complete operation and returns a result. The pipeline does not spawn goroutines, manage background workers, or coordinate concurrent access.

**How it works:**

1. Caller invokes a pipeline function (e.g., `Lookup`, `LookupAPI`, `BatchLookupStream`).
2. Pipeline executes the operation synchronously: normalise → query DB → if needed, query API → validate → ingest → return result.
3. Caller receives the result and handles it (display, store, pass to next operation).

**How it MUST work:**

- Each pipeline call is **self-contained** — it opens no long-lived state beyond the DB handle passed in.
- Pipeline functions **do not launch background goroutines** (except `BatchLookupStream`, which returns a channel the caller reads from — the goroutine is an implementation detail of the streaming pattern, not concurrent access to shared state).
- Pipeline functions **do not modify shared mutable state** — they read from the DB, normalise in local variables, and write back through the DB handle.
- Pipeline functions **block until complete** — the caller knows the operation is done when the function returns (or the channel is closed for streaming).

**How concurrency is handled externally:**

The pipeline does not manage concurrency. If a calling application needs to run multiple pipeline operations simultaneously (e.g., a GUI that searches while a batch import runs in the background), the **caller** is responsible for:

- Running pipeline calls in separate goroutines.
- Serialising access with a mutex if needed.
- Managing goroutine lifecycle and cancellation.

The pipeline's job is correctness: every call produces the right result for its inputs. The caller's job is orchestration: when and how to make those calls.

This separation keeps the pipeline simple, testable, and free of hidden side effects.

---

## Data Flow: Before and After

### Before (Previous)

```
User input ──► commands/lookup.go ──► normalize.MustNormalize(query)
                                          │
                                      ┌───┘
                                      ▼
                              db.GetMangaByTitleFuzzy()
                                  │ normalise internally
                                  │ load ALL rows
                                  │ normalise each title
                                  │ compare
                                  ▼
                              if not found:
                                  normalize.MustNormalize(query)  ← normalised AGAIN
                                  api.SearchManga(apiQuery)
                                  queryMatchesAnyTitle()          ← normalise each result title
                                  db.InsertManga()
                                      FoldUnicode(title)          ← FoldUnicode only
                                      FoldAltTitlesJSON(alt)      ← FoldUnicode only
```

Normalisation happened in 5 different places. No single point of control. This was **only for manga** — anime, lightnovel, webnovel, webtoons had no normalisation at all.

### After (Current)

```
User input ──► pipeline.Lookup(db, MediaTypeManga, "title")
                  │
                  ├── normalize.New() + MustNormalize("title")  ──► normalised query
                  │
                  ├── db.AllMedia(MediaTypeManga)
                  │       │
                  │       ├── returns raw []Media
                  │       │
                  │       ├── allTitles(each)           ← extract all language variants
                  │       ├── normalize.MustNormalize() ← normalise for comparison
                  │       └── compare + return
                  │
                  ├── if not found:
                  │       pipeline.QueryForAPI("title")  ← normalise for API search
                  │       api.SearchManga(apiQuery)
                  │       queryMatchesAnyTitle()         ← validate API results
                  │       pipeline.Ingest(media)
                  │           ├── normalize.FoldUnicode(title)
                  │           ├── normalize.NormalizeAllTitlesJSON(alt)
                  │           └── db.UpsertMedia(normalised)
                  │
                  └── return result
```

The same pipeline handles **all media types**:

```
pipeline.Lookup(db, MediaTypeAnime, "title")       ──► same flow, anime API
pipeline.Lookup(db, MediaTypeLightNovel, "title")  ──► same flow, novel API
pipeline.Lookup(db, MediaTypeWebtoon, "title")     ──► same flow, webtoon API
```

Each media type has its own API client, but the normalisation and storage flow is identical. Adding a new media type means:
1. Adding a new API client (data source).
2. Adding a new `MediaType` constant.
3. The pipeline handles the rest — normalisation, storage, lookup.

---

## Advantages

### 1. Enforceable Guarantee

With a pipeline, you can state: **"All data entering the system — for any media type — is normalised by the pipeline."** This is verifiable — you can grep for direct `data.InsertMedia` calls outside the pipeline and flag them as violations. Currently, there is no such guarantee for manga, and no guarantee at all for other media types.

### 2. Single Normalisation Strategy

The pipeline defines WHERE normalisation happens, for ALL media types. Currently, FoldUnicode is applied at write time for manga only, and full normalise at read time, with no documented rule for why. The pipeline makes this explicit and universal:

- **Write path**: Pipeline normalises before calling data component — same for manga, anime, light novel, web novel, webtoon.
- **Read path**: Pipeline normalises query, data component returns raw data, pipeline normalises for comparison — same for all media types.

### 3. Adding New Data Sources Is Safe

When a new media type is added (e.g., anime via AniList, light novels via a different API), the developer adds an API client and a `MediaType` constant. The pipeline handles normalisation and storage uniformly. Without the pipeline, each new media type would independently decide whether and how to normalise — which is exactly how the non-manga tables lost normalisation.

### 4. Independent Testing

| Component | Tests | What They Verify |
|-----------|-------|-----------------|
| Normalise | Unit tests with known I/O | `"Chainsaw.Man"` → `"chainsaw man"` |
| Data | Integration tests with real SQLite | Insert then retrieve, schema correctness |
| Pipeline | Integration tests with mocked dependencies | "API response X is normalised and stored correctly" |

Each component can be tested without the others. The normalise component has no dependencies. The data component has no normalisation logic. The pipeline orchestrates and is tested for correct wiring.

### 5. Clear Upgrade Path

When NFKC replaces the manual fold table, only the normalise component changes. The data component and pipeline remain untouched. When a new media type is added (anime, light novel, etc.), the pipeline gains a new `MediaType` constant and the data component gains a new table — but the normalisation flow is identical for all media types.

---

## Disadvantages

### 1. Significant Refactor

Every call site that currently touches the database directly must be rerouted through the pipeline. This affects:

- `commands/lookup.go` — 3 database calls + 2 normalisation calls
- `commands/batch.go` — 4 database calls + 3 normalisation calls
- `commands/maintenance.go` — 1 database calls + 1 normalisation call
- Future commands for anime, light novel, web novel, webtoon lookups

Additionally, the data component must be refactored from manga-specific methods (`InsertManga`, `GetMangaByTitle`) to media-agnostic methods (`InsertMedia`, `GetMediaByTitle`). This is a larger refactor than just rerouting calls — it changes the data model.

### 2. Read-Time Normalisation Cost

Currently, `GetMangaByTitleFuzzy` loads ALL rows and normalises every title on every query. This cost exists today and wouldn't change with the pipeline — but the pipeline makes it more visible. A future optimisation (pre-computed normalised index in the database) would reduce this, but that's a separate concern.

### 3. Pipeline Can Become a God Object

If the pipeline accumulates business logic (match selection, disambiguation, error handling), it becomes a monolithic mediator that's hard to maintain. The pipeline should be a thin orchestration layer, not a business logic layer. This requires discipline.

### 4. Over-Engineering Risk

For a CLI tool with 500 manga in the database, the pipeline adds abstraction without immediate performance benefit. The value is in **correctness guarantees** and **future scalability**, not in current performance. If the tool stays small, the pipeline may feel like unnecessary indirection.

### 5. Two Normalisation Tiers Need Clear Rules

The current system has two tiers:
- **FoldUnicode** (storage): confusable chars → ASCII, preserves case/words
- **Full normalise** (comparison): full canonical form

The pipeline must define which tier applies at each boundary for **every media type**. This is a design decision, not a technical limitation. Getting it wrong means either over-normalising stored data (losing readability) or under-normalising comparisons (missing matches). The media-agnostic design means this rule is defined once in the pipeline, not repeated per media type.

---

## Alternative Approaches

### Alternative A: Keep Normalisation in Data Component, Make It Universal

Instead of a separate pipeline, add FoldUnicode to ALL data component methods (all tables, all insert/update paths).

**Pros**: Smaller refactor, normalisation is co-located with storage.
**Cons**: Normalisation logic still lives inside the data component, mixing concerns. Adding new normalisation rules (e.g., NFKC) requires changing the data component. Testing the data component requires testing normalisation. No single point of enforcement for read-time normalisation.

### Alternative B: Middleware / Decorator Pattern

Wrap the data component with a normalising decorator:

```go
type NormalisingDB struct {
    db      *data.DB
    normalise *normalise.Normaliser
}

func (n *NormalisingDB) InsertManga(m *Manga) (int64, error) {
    m.Title = n.normalise.FoldUnicode(m.Title)
    return n.db.InsertManga(m)
}
```

**Pros**: Data component stays pure, normalisation is applied consistently.
**Cons**: Only solves the write path. Read-time normalisation (for comparison) still needs to be applied somewhere else. The decorator pattern doesn't define the full data boundary — it's a partial solution.

### Alternative C: Normalise-on-Write with Pre-Computed Index

Store a `title_normalised` column in the database. Populate it at write time with the full normalised form. Index it. Queries compare against this column instead of normalising at read time.

**Pros**: O(1) lookup instead of O(n) load-and-compare. Eliminates read-time normalisation cost.
**Cons**: Requires schema migration. The normalised column must be kept in sync (trigger or application logic). Doesn't solve the "no normalisation for non-manga tables" problem on its own — but combined with the pipeline, it would.

---

## User-Facing Layer: CLI and GUI

The end goal is two consumer applications that both use the same core libraries:

```
┌──────────────────────────────────────────────────────┐
│                  USER-FACING LAYER                    │
│                                                      │
│  ┌──────────────┐              ┌──────────────┐      │
│  │   CLI Tool   │              │  GUI App     │      │
│  │  (cobra)     │              │  (future)    │      │
│  └──────┬───────┘              └──────┬───────┘      │
│         │                             │               │
│         └──────────┬──────────────────┘               │
│                    ▼                                  │
│  ┌──────────────────────────────────────────────┐    │
│  │              CORE LIBRARIES                   │    │
│  │  pipeline/ ──► normalise/                     │    │
│  │       │                                       │    │
│  │       └──────► data/                          │    │
│  └──────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
```

### Design Principle: Library-First

The core libraries (`pipeline/`, `normalise/`, `data/`) are designed as **reusable Go packages**, not as CLI internals. The CLI and GUI are thin consumers that handle only user interaction:

| Concern | Core Libraries | CLI | GUI |
|---------|---------------|-----|-----|
| Normalisation | `normalise/` | — | — |
| Storage | `data/` | — | — |
| Data flow / boundary | `pipeline/` | — | — |
| Input parsing | — | cobra args, file reading | Form fields, file upload |
| Output formatting | — | Text / JSON to stdout | JSON over HTTP / WebSocket |
| Disambiguation | — | Interactive stdin prompt | UI modal / selection |
| Error display | — | `fmt.Fprintf(stderr)` | UI error state |
| Process lifecycle | — | Single-shot execution | Long-running server |

### CLI Layer (`commands/`)

The CLI uses cobra commands. Each command:
1. Creates a `*Pipeline` via `openPipeline()` (defined in `lookup.go`).
2. Parses user input (args, flags, files).
3. Calls the pipeline.
4. Formats and displays the result.

The CLI owns:
- **Disambiguation** — when multiple matches exist, the CLI prompts the user to pick. This is a UI concern, not a data concern.
- **Output formatting** — text, JSON, verbose modes. This is presentation, not data.
- **File parsing** — reading `.txt` and `.csv` files for batch input. This is I/O, not data normalisation.
- **Exit codes and error messages** — CLI-specific conventions.

The CLI does NOT own:
- Normalisation logic.
- Database operations.
- Data flow decisions.

### GUI Layer (Future)

The GUI will be a separate binary, likely a web-based interface (e.g., Go HTTP server + browser frontend, or a desktop framework like Wails/Fyne).

Two possible architectures:

#### Option A: Embedded Server

The GUI binary embeds the core libraries and runs an HTTP server:

```
Browser ──► HTTP Server (Go) ──► pipeline/ ──► data/
                                    │
                                    └──► normalise/
```

**Pros**: Single binary, no external dependencies, simple deployment.
**Cons**: GUI and server are coupled in one process. Can't serve multiple clients.

#### Option B: Separate Server + Client

A standalone API server exposes the pipeline over HTTP/REST. The GUI is a thin client:

```
Browser/GUI ──► HTTP API Server ──► pipeline/ ──► data/
                                    │
                                    └──► normalise/
```

**Pros**: Multiple clients (CLI, GUI, scripts, other tools). Clean separation.
**Cons**: Two processes to manage. Requires API design.

**Recommendation**: Start with Option A (embedded). The core libraries are already designed as importable packages — the HTTP server is just another consumer of `pipeline/`. If multi-client support is needed later, extract the server into its own binary.

### Advantages of Library-First Design

#### 1. Same Guarantee for All Interfaces

Whether the user runs `scrape lookup "title"` from the CLI or searches via the GUI, the data flows through the same `pipeline.Lookup()` function. Normalisation is applied identically. There is no risk of the GUI implementing its own normalisation logic and diverging from the CLI.

#### 2. Testability Across Consumers

The core libraries can be tested independently of any UI. The CLI can be tested with mock pipelines. The GUI can be tested with mock pipelines. No UI framework is needed to test data correctness.

#### 3. Future-Proof

Adding a new consumer (e.g., a REST API, a TUI, an integration with another tool) requires only importing the core packages. No changes to the pipeline, normalise, or data layers.

#### 4. CLI and GUI Can Evolve Independently

The CLI can add new commands without affecting the GUI. The GUI can add new views without affecting the CLI. Both are thin wrappers around the same core.

### Disadvantages of Library-First Design

#### 1. API Surface Must Be Designed Carefully

The pipeline API (`Lookup`, `BatchLookup`, `Ingest`, `NormalizeAll`) must be general enough for both CLI and GUI, but not so abstract that it loses type safety. If the GUI needs operations the CLI doesn't (e.g., paginated listing, search-as-you-type), the pipeline API must accommodate them without becoming bloated.

The pipeline returns raw data types (`*Media`, `[]*BatchResult`, etc.). Each consumer wraps these in its own presentation logic — the CLI formats text to stdout, the GUI renders in a browser. No shared response envelope.

#### 2. State Management Differences

The CLI is single-shot: open DB, run command, close DB. The GUI is long-lived: the database connection stays open, normaliser instances are reused. The `Pipeline` struct handles both patterns — create one `New(db, opts)` at startup and reuse it for the process lifetime. The normaliser is constructed once inside the Pipeline and shared across all calls.

#### 3. Concurrency

The pipeline does not manage concurrency. The CLI is single-threaded per invocation. A GUI can use goroutines and channels (e.g., `BatchLookupStream`) to keep the UI responsive during long operations. If concurrent access to the pipeline is needed, the caller manages it (mutex, serialised goroutine, etc.) — this is a presentation-layer concern, not a data-layer concern.

#### 4. Error Handling Differs

CLI errors go to stderr and exit with a non-zero code. GUI errors must be serialised (JSON error responses) and mapped to UI states. The pipeline returns structured errors (`PipelineError` with `Kind ErrorKind`, `Message`, `Err`) — callers use `errors.As` to extract the kind and handle it appropriately.

#### 5. Output Formatting Is Not in the Core

The pipeline returns raw data structs. The CLI formats them as text/JSON to stdout. The GUI renders them in a browser. This is correct separation — the core libraries return complete, well-structured data, and each consumer decides what to display.

---

## Recommendation

The three-pillar architecture is the right approach for this codebase, for these reasons:

1. **The core guarantee is correctness, not performance.** The pipeline enforces that all data is normalised, which is the stated priority.

2. **The refactor cost is moderate.** The codebase is ~2,000 lines of Go. The data flows are well-defined (4 commands). Rerouting through a pipeline is mechanical.

3. **The alternative (keep normalisation in data component) doesn't solve the read-time problem.** FoldUnicode at storage time is not enough — comparisons require full normalisation, and that logic currently lives in the command layer. The pipeline unifies both paths.

4. **The upgrade path is clear.** NFKC refactoring, expanding to non-manga tables, adding new data sources — all become straightforward changes within the pipeline, rather than scattered edits across commands and the database layer.

### Implementation Order

1. ~~**Define media type abstraction**~~ — `MediaType` constants and `Media` struct in data component. **Done.**
2. ~~**Extract data component**~~ — normalisation removed from `internal/database/`. All methods media-agnostic. **Done.**
3. ~~**Create pipeline component**~~ — `internal/pipeline/` with `Lookup`, `BatchLookup`, `Ingest`, `NormalizeAllTitles`. **Done.**
4. ~~**Move normalisation from commands to pipeline**~~ — all commands call pipeline, no direct `normalize.*` calls from commands. **Done.**
5. ~~**Add normalisation to non-manga tables**~~ — pipeline handles all 5 title-bearing tables uniformly. **Done.**
6. ~~**Remove fuzzy matching**~~ — removed entirely; normalisation handles systematic variants, API handles typos. **Done.**
7. ~~**Refactor CLI as thin consumer**~~ — CLI imports only `pipeline/` and `database/`. No direct `mangadex/`, `ratelimit/`, or `normalize/` imports from commands. **Done.**
8. ~~**Pipeline struct + structured errors**~~ — `Pipeline` struct with `Options`, `PipelineError`/`ErrorKind` types, `LookupService` interface for testing. All methods are struct methods on `*Pipeline`. **Done.**
9. ~~**Pre-computed title index**~~ — `title_index` table with `(media_type, normalised)` index. Pipeline maintains it at ingest/normalize time. `Lookup` and `BatchLookup` use indexed queries instead of full table scans. Auto-rebuilds on startup for existing databases. **Done.**
10. **Build GUI** — import `pipeline/`, implement HTTP server or desktop UI. No changes to core libraries needed.

---

## Package Dependency Rules

```
CLI (commands/) ──┐
                   ├──► pipeline/ ──► normalise/
GUI (future)   ──┘         │
                           └──────► data/ ──► database drivers
```

- **CLI** (`commands/`) depends on `pipeline/`. Never on `normalise/` or `data/` directly.
- **GUI** (future) depends on `pipeline/`. Same rules as CLI.
- **`pipeline/`** depends on `normalise/` and `data/`. Never on `commands/` or GUI code.
- **`normalise/`** depends on `golang.org/x/text` (NFKC, case fold). No other dependencies.
- **`data/`** depends on database drivers. Does NOT depend on `normalise/`.

The key structural constraint: `normalise/` and `data/` have zero dependency on each other. `pipeline/` is the only package that imports both. This enforces the data boundary.

Both CLI and GUI are **consumers** of the core libraries. They import `pipeline/` but never import `normalise/` or `data/` directly. This ensures that all data flows through the pipeline boundary, regardless of which user-facing application is running.
