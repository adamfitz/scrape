# database-maintenance Specification

## Purpose

Perform SQLite database maintenance operations: VACUUM (compact), integrity checks, title normalization, and optimization.

## Requirements

### Requirement: CLI command

The tool SHALL provide a `scrape maintenance` command with subcommands.

#### Scenario: VACUUM

- GIVEN the command `scrape maintenance vacuum`
- WHEN executed
- THEN the database SHALL be compacted via SQLite VACUUM
- AND the database file size SHALL be reduced (if fragmentation exists)

#### Scenario: Integrity check

- GIVEN the command `scrape maintenance check`
- WHEN executed
- THEN the database integrity SHALL be verified
- AND results SHALL be displayed (ok or corruption detected)

#### Scenario: Stats

- GIVEN the command `scrape maintenance stats`
- WHEN executed
- THEN database statistics SHALL be displayed (table counts, sizes, page counts)

#### Scenario: Normalize titles

- GIVEN the command `scrape maintenance normalize`
- WHEN executed
- THEN all manga titles and alt_titles SHALL be Unicode-folded (confusable characters → ASCII)
- AND the count of updated records SHALL be displayed
- AND running the command again SHALL update 0 records (idempotent)

### Requirement: VACUUM safety

VACUUM SHALL NOT be performed if the database is below 1MB (unnecessary overhead).

#### Scenario: Small database

- GIVEN a database smaller than 1MB
- WHEN `maintenance vacuum` is executed
- THEN VACUUM SHALL be skipped with a message indicating the database is already compact

### Requirement: Integrity check using PRAGMA

The integrity check SHALL use `PRAGMA integrity_check`.

#### Scenario: Healthy database

- GIVEN a non-corrupt database
- WHEN integrity check runs
- THEN the result SHALL be "ok"

#### Scenario: Corrupt database

- GIVEN a corrupt database
- WHEN integrity check runs
- THEN the result SHALL detail the corruption

### Requirement: Stats output

Stats SHALL include:
- Total database file size
- Number of records per table
- WAL file size (if present)

#### Scenario: Stats display

- GIVEN a database with 100 manga records
- WHEN stats are displayed
- THEN output SHALL include `manga: 100 records`

### Requirement: Title normalization

The `normalize` subcommand SHALL re-normalise all `title` and `alt_title` values in the manga table using Unicode folding. This is a one-time migration for data inserted before Unicode folding was applied. The operation is idempotent.

#### Scenario: First run normalizes legacy data

- GIVEN the database has 200 manga records with raw Unicode characters in titles
- WHEN `scrape maintenance normalize` is executed
- THEN all titles SHALL be Unicode-folded
- AND the output SHALL show `Updated N title(s)` where N > 0

#### Scenario: Second run is no-op

- GIVEN the database was just normalized
- WHEN `scrape maintenance normalize` is executed again
- THEN the output SHALL show `Updated 0 title(s)`
