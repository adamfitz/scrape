# sqlite-database Specification

## Purpose

Persistent storage for manga metadata, reading observations, bookmarks, and future media types. Uses pure-Go SQLite (`modernc.org/sqlite`) with no CGO dependency.

## Requirements

### Requirement: Database location

The database SHALL be stored at `~/.config/scrape/scrape.db` (or the platform-equivalent config directory).

### Requirement: Schema creation

The database SHALL auto-create all tables on first open if they do not exist.

### Requirement: Manga table schema

```sql
CREATE TABLE IF NOT EXISTS manga (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT NOT NULL,
    alt_title   TEXT,  -- JSON: structured format (see below)
    author      TEXT,
    description TEXT,
    cover_url   TEXT,
    url         TEXT,
    mangadex_id TEXT UNIQUE,
    status      TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### alt_title JSON format

The `alt_title` column stores a structured JSON object with primary titles and the full alt titles array:

```json
{
  "primary": {"en": "One Piece", "ja": "ワンピース"},
  "alts": [
    {"en": "One Piece"},
    {"ja": "ワンピース"},
    {"ja-ro": "Wan Pīsu"}
  ]
}
```

- `primary`: the MangaDex primary title map (highest priority per language)
- `alts`: the full `altTitles` array from MangaDex (preserves all variants including sub-story names, multiple translations per language)

The old flat format `{"en": "...", "ja": "..."}` is supported as a fallback for backward compatibility.

### Requirement: Observation table schema

```sql
CREATE TABLE IF NOT EXISTS observation (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id    INTEGER NOT NULL,
    media_type  TEXT NOT NULL,
    progress    TEXT,
    title       TEXT,
    url         TEXT,
    status      TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Requirement: Bookmarks table schema

```sql
CREATE TABLE IF NOT EXISTS bookmarks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id    INTEGER NOT NULL,
    media_type  TEXT NOT NULL,
    chapter_id  INTEGER,
    note        TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Requirement: Future media tables

anime, lightnovel, webnovel, webtoons — all with standard column convention (id, title, alt_title, url, status, created_at, updated_at).

### Requirement: Normalized title lookup (title index)

Lookups SHALL use a pre-computed `title_index` table for O(log N) indexed queries. The pipeline maintains the index automatically — at ingest time, each title variant is normalised and stored in the index. At lookup time, the query is normalised and the index is searched. This replaces the previous in-memory full-table-scan approach.

#### Scenario: Punctuation variant match

- GIVEN DB has title "After Being Called a Hero: The Unrivaled Man Starts a Family"
- WHEN querying "After Being Called a Hero The Unrivaled Man Starts a Family"
- THEN the record SHALL be found via the title index (normalised forms match)

#### Scenario: Alt title match across languages

- GIVEN DB has manga with primary `{"ja": "コワモテ高校生とジミコさん"}` and alt titles including `{"en": "Scary Face High Schooler and Miss Plain Jane"}`
- WHEN querying "Scary Face High Schooler and Miss Plain Jane"
- THEN the record SHALL be found because the alt title is indexed separately

#### Scenario: Case-insensitive match

- GIVEN DB has title "One Piece"
- WHEN querying "one piece"
- THEN the record SHALL be found via the title index (normalised forms match)

#### Scenario: Batch normalized matching

- GIVEN 500 manga in the database
- WHEN 100 titles are batch-checked
- THEN all 100 query titles SHALL be normalised and looked up in the index in a single SQL query

### Requirement: GetMangaByMangadexID

Exact lookup by MangaDex ID (no normalization needed).

### Requirement: InsertManga

Inserts a new manga record. The title is Unicode-folded (confusable characters mapped to ASCII equivalents) before storage. The alt_title JSON values are also Unicode-folded via `FoldAltTitlesJSON`. This ensures all stored titles use canonical character forms, preventing mismatches caused by visually-similar Unicode variants.

### Requirement: UpdateManga

Updates an existing manga record by ID. Title and alt_title are Unicode-folded before storage, matching the InsertManga behavior.

### Requirement: NormalizeAllTitles

The database SHALL provide a `NormalizeAllTitles()` method that re-normalises every `title` and `alt_title` in the manga table using Unicode folding. This is an idempotent migration operation — running it multiple times produces no additional changes. It returns the number of rows updated.

#### Scenario: Legacy data migration

- GIVEN the database contains records inserted before Unicode folding was applied
- WHEN `NormalizeAllTitles()` is executed
- THEN all titles and alt_titles SHALL be updated with Unicode-folded values
- AND subsequent calls SHALL update 0 rows (idempotent)

### Requirement: Upsert by MangaDex ID

When inserting a manga, if a record with the same `mangadex_id` already exists, the existing record SHALL be updated with the new data instead of failing with a UNIQUE constraint error.

### Requirement: Updated_at trigger

The `updated_at` field SHALL be automatically set to the current timestamp on UPDATE operations.

## Implementation Notes

- Use `modernc.org/sqlite` (pure Go, no CGO)
- Use `database/sql` standard interface
- WAL mode for better concurrent read performance
- All queries use parameterized statements (no SQL injection)
- Title index stores normalised forms at ingest time for O(log N) lookups
- Index is automatically rebuilt on pipeline startup for existing databases
