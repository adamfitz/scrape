# fuzzy-matching Specification — REMOVED

## Status: Removed

Fuzzy matching has been removed from the application. The `internal/fuzzy/` package no longer exists.

## Rationale

The normalisation pipeline (NFKC + supplemental fold + separators + noise + stop words + Unicode case fold) handles all systematic title variations. Typos and spelling errors are handled by the MangaDex API — when local normalised lookup fails, the pipeline queries MangaDex, which accepts fuzzy/approximate queries natively.

Adding a local fuzzy matcher on top of comprehensive normalisation was redundant and introduced an unnecessary dependency (`adrg/strutil`).

## What was removed

- `internal/fuzzy/` package (Sorensen-Dice similarity)
- `--fuzzy` flag from `lookup` and `batch` commands
- `LookupFuzzy`, `BatchLookupFuzzy`, `BatchLookup` wrapper from `internal/pipeline/pipeline.go`
- `GetMangaByTitleFuzzy`, `BatchCheckTitlesFuzzy` from `internal/database/database.go`

## See also

- `name-normalization/spec.md` — the normalisation pipeline that replaces fuzzy matching
- `architecture-analysis.md` — rationale for the three-pillar architecture
