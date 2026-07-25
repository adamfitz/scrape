# single-lookup Specification

## Purpose

Look up a single manga title via the CLI, checking the local database first before querying MangaDex.

## Requirements

### Requirement: CLI command

The tool SHALL provide a `scrape lookup <title>` command.

#### Scenario: Basic lookup

- GIVEN the command `scrape lookup "One Piece"`
- WHEN executed
- THEN the tool SHALL normalize the title
- AND check the local database
- AND if not found, query MangaDex
- AND display the result

### Requirement: Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | | false | Output results in JSON format |
| `--verbose` | `-v` | false | Show full details (alt titles, author, cover, description) |
| `--local` | `-l` | false | Only check the local database, skip the API |
| `--limit` | | 5 | Max MangaDex results to consider |

### Requirement: Local-first strategy

The database SHALL be checked before any API calls.

#### Scenario: Title found locally

- GIVEN "One Piece" exists in the local database
- WHEN `scrape lookup "One Piece"` is executed
- THEN no API call SHALL be made
- AND the cached result SHALL be displayed with `[db]` source indicator

#### Scenario: Title not found locally

- GIVEN "One Piece" does not exist in the local database
- WHEN `scrape lookup "One Piece"` is executed
- THEN a MangaDex API call SHALL be made
- AND the result SHALL be stored in the local database
- AND the result SHALL be displayed with `[api]` source indicator

### Requirement: Local-only mode

When `--local` is set, the API SHALL NOT be called.

#### Scenario: --local flag with found title

- GIVEN "One Piece" exists in the local database
- WHEN `scrape lookup "One Piece" -l` is executed
- THEN the cached result SHALL be displayed
- AND no API call SHALL be made

#### Scenario: --local flag with missing title

- GIVEN "Unknown Manga" does not exist in the local database
- WHEN `scrape lookup "Unknown Manga" -l` is executed
- THEN output SHALL be `[db] Unknown Manga — not found locally`
- AND no API call SHALL be made

### Requirement: Default output (minimal)

The default output SHALL show: source indicator and English title only.

#### Scenario: Minimal output

- GIVEN a lookup returns "One Piece" from MangaDex
- WHEN displayed in minimal format
- THEN output SHALL be `[api] One Piece`

### Requirement: Verbose output

When `-v` is set, the output SHALL show all available details.

#### Scenario: Verbose output

- GIVEN a lookup with `-v` flag
- WHEN displayed
- THEN output SHALL include Source, Title, Alt Titles, Author, Status, Language, MangaDex URL, Cover URL, Description

### Requirement: JSON output

When `--json` is set, the output SHALL be a valid JSON object with all fields.

### Requirement: Auto-store successful lookups

Successful MangaDex lookups SHALL be automatically stored in the local database.

#### Scenario: New lookup stored

- GIVEN a MangaDex lookup returns a valid result
- WHEN the lookup completes
- THEN the manga record SHALL be inserted into the local database

### Requirement: Source indicator

Every output line SHALL indicate whether the result came from the local database (`[db]`) or the MangaDex API (`[api]`).

### Requirement: English title resolution

The default output SHALL display the English title from the primary title map when available.

#### Scenario: Japanese input, English output

- GIVEN the query `"鬼滅の刃"`
- WHEN MangaDex returns English title "Demon Slayer"
- THEN the output SHALL display `Demon Slayer`

### Requirement: Normalized API queries

Queries sent to MangaDex SHALL be normalized before the API call (Unicode folded, periods/underscores/separators folded to spaces, noise removed, lowercased).

### Requirement: Unicode-aware storage

Titles retrieved from the MangaDex API SHALL be Unicode-folded (confusable characters → ASCII) before storage in the local database. This ensures that subsequent lookups with different Unicode variants of the same character (e.g., `'` vs `'`, `～` vs `~`) will match correctly.

### Requirement: Upsert by MangaDex ID

When a manga is found via the MangaDex API, the tool SHALL check if a record with the same `mangadex_id` already exists. If it exists, the existing record SHALL be updated with fresh API data. If not, a new record SHALL be inserted.

### Requirement: Result validation

The tool SHALL NOT blindly accept the first MangaDex result. The tool SHALL iterate through the returned results and only accept a result where the normalized query appears as a substring in at least one of the result's titles (primary + alt titles, after normalization). If no result matches, the tool SHALL report "no matching result on MangaDex" and SHALL NOT store anything.

#### Scenario: MangaDex returns wrong match

- GIVEN the input title "You Like Me, Don't You"
- WHEN MangaDex returns "I Want to Hear You Say You Like Me" (a different manga)
- AND the normalized query does not appear in any normalized title of the result
- THEN the result SHALL be rejected
- AND the output SHALL be `[api] You Like Me, Don't You — no matching result on MangaDex`

#### Scenario: MangaDex returns correct match among several results

- GIVEN the input title "your name"
- WHEN MangaDex returns 5 results
- AND the first result has no matching title
- AND the second result has "Your Name." as an alt title
- THEN the second result SHALL be accepted
- AND no further results SHALL be checked
