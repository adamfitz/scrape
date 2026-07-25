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
- AND the summary SHALL show: Found (cached), Not found
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
Summary: Found: N, Cached: N, Not found: N
```

- `Found` = total found (cached + API)
- `Cached` = found in local database
- `Not found` = not found anywhere

### Requirement: Source indicator

Each result line SHALL indicate `[db]` or `[api]`.

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

The tool SHALL NOT blindly accept the first MangaDex result. For each title, the tool SHALL iterate through the returned results and only accept a result where the normalized query appears as a substring in at least one of the result's titles (primary + alt titles, after normalization). If no result matches, the title SHALL be reported as not found and SHALL NOT be stored.

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
