# batch-lookup Specification

## Purpose

Look up multiple manga titles from text or CSV files, with local-first lookup to minimize API calls.

## Requirements

### Requirement: CLI command

The tool SHALL provide a `scrape batch <file>` command.

### Requirement: Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | | false | Output results in JSON format |
| `--verbose` | `-v` | false | Show full details |
| `--local` | `-l` | false | Only check the local database, skip the API |
| `--limit` | | 5 | Max MangaDex results per title |
| `--show-unmatched` | | false | Show titles not added to the DB (API rejected or not found) |
| `--diff` | | false | Show input file entries missing from the database (implies `--local`) |

### Requirement: Input formats

#### Text file format

Text files SHALL have one title per line. Empty lines and lines starting with `#` SHALL be ignored.

#### CSV file format

CSV files SHALL have a header row. The first column SHALL be treated as the title.

### Requirement: Normalized deduplication

Duplicate titles SHALL be detected using normalized forms, not raw strings.

#### Scenario: Punctuation variants

- GIVEN input contains "After Being Called a Hero: The Unrivaled Man Starts a Family" and "After Being Called a Hero The Unrivaled Man Starts a Family"
- WHEN deduplicated
- THEN only ONE entry SHALL be processed

#### Scenario: Case variants

- GIVEN input contains "One Piece" and "one piece"
- WHEN deduplicated
- THEN only ONE entry SHALL be processed

### Requirement: Local-first batch strategy

ALL titles SHALL be checked against the local database before ANY API calls are made. Normalized matching is used for the DB check.

#### Scenario: Mixed local and remote

- GIVEN 10 titles input
- AND 7 exist in the local database (matched by normalized form)
- WHEN batch processing starts
- THEN the 7 local results SHALL be returned immediately
- AND only 3 MangaDex API calls SHALL be made

### Requirement: Local-only mode

When `--local` is set, the API SHALL NOT be called. Only the local database is queried.

#### Scenario: --local with 721 titles

- GIVEN 721 titles input
- WHEN `scrape batch titles.txt -l` is executed
- THEN all 721 titles SHALL be checked against the local database
- AND the summary SHALL show DB, Fuzzy, ambiguous, and not-found counts
- AND NO API calls SHALL be made

### Requirement: Progress display

The tool SHALL display progress during batch processing.

### Requirement: Rate-limited API calls

Batch API calls SHALL go through the rate limiter (4 req/s default).

### Requirement: Error resilience

Individual title failures SHALL not abort the batch.

### Requirement: Summary

After processing, a summary line SHALL be displayed:

```
---
713 processed — 630 found (DB: 600, Fuzzy: 30, API: 0), 11 ambiguous, 47 not found
  → Ambiguous: run `scrape lookup "<title>"` for each to disambiguate
  → Not found: not on MangaDex — check manually or skip
---
```

- `DB` = exact match via title_index
- `Fuzzy` = single fuzzy match auto-linked via `LinkQuery` (reuses existing record)
- `API` = found and ingested from MangaDex
- `ambiguous` = multiple fuzzy candidates — user must disambiguate via `scrape lookup`
- `not found` = not found in DB or MangaDex

Actionable hints SHALL be printed when ambiguous or not-found counts are > 0.

### Requirement: Source indicator

Each result line SHALL indicate `[db]`, `[fuzzy]`, or `[api]`.

- `[db]` — exact match via title_index
- `[fuzzy]` — single fuzzy match (auto-linked) or `[fuzzy] [fuzzy multiple]` for ambiguous matches
- `[api]` — match via MangaDex API

### Requirement: Default output (minimal)

The default output SHALL show: source indicator and English title only (`[source] Title`).

### Requirement: Verbose output

When `-v` is set, full details are shown per entry (Source, Title, Alt Titles, Author, Status, MangaDex URL, Description).

### Requirement: JSON output

When `--json` is set, the output SHALL be a JSON array of lookupOutput objects.

### Requirement: Normalized API queries

Queries sent to MangaDex SHALL be normalized (Unicode folded, periods/underscores/separators folded to spaces, noise removed, lowercased) before the API call. This ensures MangaDex receives clean search terms (e.g., "Akame.ga.KILL" → "akame ga kill").

### Requirement: Unicode-aware storage

Titles retrieved from the MangaDex API SHALL be Unicode-folded before storage, ensuring consistent character forms in the database regardless of which Unicode variant was in the input file or API response.

### Requirement: Upsert by MangaDex ID

When a manga is found via the MangaDex API, the tool SHALL check if a record with the same `mangadex_id` already exists in the database. If it exists, the existing record SHALL be updated with the fresh API data. If not, a new record SHALL be inserted.

### Requirement: Result validation

The tool SHALL NOT blindly accept the first MangaDex result. For each title, the tool SHALL iterate through the returned results and only accept a result where the normalized query appears as a substring in at least one of the result's titles (primary + alt titles, after normalization). If no result matches, the tool SHALL attempt fuzzy similarity matching (threshold >= 0.85) as a 4th strategy. If no result matches even fuzzily, the title SHALL be reported as not found and SHALL NOT be stored.

#### Scenario: MangaDex returns wrong match

- GIVEN the input title "You Like Me, Don't You"
- WHEN MangaDex returns "I Want to Hear You Say You Like Me" (a different manga)
- AND the normalized query "you like me don't you" does not appear in any normalized title of the result
- THEN the result SHALL be rejected
- AND the title SHALL be reported as "not found (no match)"

#### Scenario: MangaDex returns correct match

- GIVEN the input title "your name"
- WHEN MangaDex returns "What's Your Name?" which has alt title "Your Name."
- AND the normalized query "your name" appears in the normalized alt title "your name"
- THEN the result SHALL be accepted and stored

### Requirement: --diff mode

When `--diff` is set, the tool SHALL check all input entries against the local database and print only the entries that are NOT in the database. No API calls SHALL be made. This is useful for checking coverage between an input file and the database.

#### Scenario: --diff shows missing entries

- GIVEN the database contains "One Piece" and "Naruto"
- AND the input file contains "One Piece", "Naruto", and "Bleach"
- WHEN `scrape batch titles.txt --diff` is executed
- THEN the output SHALL contain only "Bleach"
- AND no API calls SHALL be made

### Requirement: --show-unmatched flag

When `--show-unmatched` is set, the tool SHALL print a section after the summary listing all titles that were not added to the database. This includes titles where the API returned no results, titles where the API returned results but none matched (rejected by result validation), and (in `--local` mode) titles not found in the database.

#### Scenario: --show-unmatched with API errors

- GIVEN 3 titles were queried via the API
- AND 1 was found and stored
- AND 1 had no results on MangaDex
- AND 1 had results but none matched
- WHEN the batch completes
- THEN the unmatched section SHALL list the 2 titles that were not added

#### Scenario: --show-unmatched with --local

- GIVEN `--local` and `--show-unmatched` are both set
- WHEN the batch completes
- THEN the unmatched section SHALL list all titles not found in the local database

### Requirement: Fuzzy fallback for missing titles

After the exact-match batch query, titles not found in the title_index SHALL be checked against all stored titles using fuzzy similarity scoring. The fuzzy scan runs before any API calls.

- **Single fuzzy match** (score >= 0.85): auto-link via `LinkQuery` and emit `LookupResult{Media, Source: SourceFuzzy}`. The existing record is reused — no new ingestion.
- **Multiple fuzzy matches** (score >= 0.85): emit `LookupResult{Source: SourceFuzzy, Candidates: [...]}` for user disambiguation.
- **No fuzzy matches**: title is sent to the MangaDex API (unless `--local` mode).

#### Scenario: Single fuzzy match auto-link

- GIVEN the database stores `"Yuusha Party"` (media_id=X)
- AND the input contains `"Yusha Party"` (typo)
- WHEN batch processes this title
- THEN tier 1 exact match SHALL miss
- AND fuzzy scan SHALL find the match (score >= 0.85)
- AND `LinkQuery` SHALL add index entry for the user's query
- AND the existing record SHALL be reused (no new ingestion)
- AND the result SHALL be displayed as `[fuzzy] Yuusha Party`

#### Scenario: Multiple fuzzy matches require disambiguation

- GIVEN the database stores two titles similar to the input
- WHEN batch processes this title
- AND fuzzy scan finds multiple matches above threshold
- THEN the batch SHALL pause
- AND display the candidates with scores
- AND wait for user selection
- AND after selection, link the result via `LinkQuery()`

### Requirement: Fuzzy disambiguation prompt

When multiple fuzzy candidates are found, the batch command SHALL display:

```
[N/M] Multiple matches for: <title>

  [1] <candidate 1 title> (score: 0.XX)
  [2] <candidate 2 title> (score: 0.XX)
  [0] Skip

Pick a number:
```

The batch SHALL wait for user input before continuing.

### Requirement: Idempotent batch runs

After the first successful batch run, subsequent runs with the same file SHALL find all titles via exact match (tier 1) — no fuzzy scanning, no API calls, no user interaction. The `LinkQuery` mechanism ensures that fuzzy-matched titles from previous runs are findable via exact match.

#### Scenario: Second run is non-interactive

- GIVEN the first batch run linked all fuzzy-matched titles via `LinkQuery()`
- WHEN the same batch file is processed again
- THEN ALL titles SHALL be found via `[db]` exact match
- AND NO fuzzy candidates SHALL be returned
- AND NO user prompts SHALL appear
- AND the batch SHALL complete non-interactively
