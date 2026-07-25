# database-backup Specification

## Purpose

Create compressed backups of the SQLite database to a user-specified destination.

## Requirements

### Requirement: CLI command

The tool SHALL provide a `scrape backup <destination>` command.

#### Scenario: Backup to file

- GIVEN the command `scrape backup ~/backups/scrape-2025-01-15.db.gz`
- WHEN executed
- THEN the database SHALL be copied and gzip-compressed to the destination path

#### Scenario: Backup to directory

- GIVEN the command `scrape backup ~/backups/`
- WHEN executed
- THEN the database SHALL be saved as `~/backups/scrape-YYYY-MM-DD-HHMMSS.db.gz`

### Requirement: Safe backup (no corruption)

The backup SHALL use SQLite's backup API or a consistent snapshot to avoid corruption.

#### Scenario: Active database backup

- GIVEN the database is open and in use
- WHEN a backup is initiated
- THEN the backup file SHALL be a consistent, non-corrupt copy
- AND the original database SHALL NOT be locked during backup

### Requirement: Compression

Backups SHALL be gzip-compressed by default.

#### Scenario: Compressed backup

- GIVEN a 50MB database
- WHEN backed up
- THEN the backup file SHALL be gzip-compressed
- AND the file SHALL end with `.gz`

### Requirement: Progress display

The tool SHALL display backup progress.

#### Scenario: Backup progress

- GIVEN a 50MB database
- WHEN backing up
- THEN progress SHALL be displayed (bytes copied / total)

### Requirement: Destination validation

The tool SHALL validate the destination path before starting.

#### Scenario: Read-only destination

- GIVEN a destination path that is not writable
- WHEN a backup is attempted
- THEN an error SHALL be displayed before any data is copied

## Implementation Notes

- Use SQLite's online backup API via `database/sql` or VACUUM INTO
- Alternative: Use `VACUUM INTO 'path'` (SQLite 3.27.0+) for atomic backup
- Gzip compression via `compress/gzip` standard library
- Generate timestamp filenames with `time.Now().Format("2006-01-02-150405")`
