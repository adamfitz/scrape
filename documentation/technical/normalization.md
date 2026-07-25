# Name Normalisation

## Overview

Normalisation transforms raw manga/novel titles into deterministic canonical forms so that two titles referring to the same work always produce the same string. This enables reliable lookups and deduplication regardless of how a title is formatted in the wild.

There are two levels of normalisation used in the codebase:

| Level | Purpose | Applied At |
|-------|---------|------------|
| **NFKC normalization** | Unicode normalization form KC — decomposes and recomposes characters to canonical equivalents, maps compatibility characters (fullwidth, ligatures, typographic symbols) to ASCII. Preserves case, words, and non-Latin scripts. | Storage boundaries (insert/update rows) |
| **Full normalisation** | Produce a case-folded, stripped, noise-free canonical form for comparison. | Query time, deduplication, API calls, result matching |

---

## Table of Contents

- [Overview](#overview)
- [When It Is Applied](#when-it-is-applied)
- [Input / Output Examples](#input--output-examples)
- [Technical Implementation](#technical-implementation)
  - [Architecture](#architecture)
  - [Full Normalisation Pipeline](#full-normalisation-pipeline)
  - [NFKC Normalization](#nfkc-normalization)
  - [Alt Titles JSON Functions](#alt-titles-json-functions)
  - [Result Validation](#result-validation)
  - [Configuration](#configuration)
- [Pipeline Diagram](#pipeline-diagram)

---

## When It Is Applied

- **On write** (`InsertManga`, `UpdateManga`): NFKC normalization only. Titles are stored in a readable, canonical-Unicode form.
- **On read** (`GetMangaByTitle`, `GetMangaByTitleFuzzy`, `BatchCheckTitlesFuzzy`): Full normalisation is applied to both the query and every stored title before comparison.
- **On API calls**: User queries are fully normalised before being sent to MangaDex.
- **On deduplication** (`batch.go`): Input titles are fully normalised to detect duplicates.
- **On maintenance** (`scrape maintenance normalize`): Re-applies NFKC normalization to every row idempotently.

## Input / Output Examples

| Input | Full Normalised Output |
|-------|----------------------|
| `"Chainsaw.Man"` | `"chainsaw man"` |
| `"[ScanGroup] Bloom Into You (Digital)"` | `"bloom into you"` |
| `"The Manga Name"` | `"manga name"` |
| `"One.Piece.Ch.1090.cbz"` | `"one piece"` |
| `"鬼滅の刃"` | `"鬼滅の刃"` (CJK preserved) |
| `"외모지상주의"` | `"외모지상주의"` (Hangul preserved) |
| `"Hero\u2019s Party"` | `"heros party"` |
| `"Ｍｏｎｓｔｅｒ ＃８"` | `"monster #8"` (fullwidth → ASCII via NFKC) |
| `"straße"` | `"strasse"` (ß → ss via case fold) |

## Technical Implementation

### Architecture

The normaliser is implemented in `internal/normalize/normalize.go`. It depends on `golang.org/x/text/unicode/norm` for NFKC normalization and `golang.org/x/text/cases` for Unicode case folding. All regexes are compiled once in `New()`. The normaliser is immutable after construction — safe for concurrent use by multiple goroutines.

### Full Normalisation Pipeline

The `Normalize()` method applies these steps in order:

1. **NFKC normalize + supplemental fold** -- Unicode NFKC via `golang.org/x/text/unicode/norm`, then a supplemental fold map for ~25 characters NFKC misses (curly quotes, en/em dashes, wave dashes, typographic symbols).
2. **Extension strip** -- Remove common media file extensions (`.cbz`, `.mkv`, `.epub`, etc.) using a pre-compiled regex.
3. **Separator fold** -- Replace punctuation separators (`.`, `_`, `-`, `:`, `;`, `,`, `!`, `?`, `'`, `"`, `~`, and their fullwidth variants) with spaces via a dynamically built character-class regex.
4. **Noise removal** -- Strip bracketed tags `[...]`, parenthesised notes `(...)`, and volume/chapter/part markers (e.g. `Vol. 3`, `Ch 1090`, `Part 2`) using a case-insensitive combined regex.
5. **Multi-space collapse** -- Repeatedly collapse double spaces into single spaces.
6. **Stop word removal** -- Filter out common words (`the`, `a`, `an`, `and`, `of` by default) from the word list.
7. **Unicode case fold** -- Case-insensitive folding via `golang.org/x/text/cases`. Replaces `strings.ToLower` to handle locale-aware case mappings (`ß → ss`, Kelvin `K → k`, etc.).

### NFKC Normalization

NFKC (Unicode Normalization Form KC) is the foundation of the fold step. It is the recommended normalization form for this codebase because:

- **Standards-compliant**: Covers all compatibility equivalents in Unicode (thousands of characters vs ~90 hand-picked entries).
- **Hangul-safe**: Unlike NFKD, NFKC does not decompose Hangul syllables into Jamo.
- **Idempotent by definition**: Applying NFKC multiple times produces the same result.
- **Handles cases the manual table missed**: Ligatures (`ﬁ` → `fi`), superscripts (`²` → `2`), mathematical symbols, and any future Unicode additions.

However, NFKC only handles **compatibility equivalents** (fullwidth → ASCII, ligatures → expanded, etc.). It does NOT handle **semantically similar characters** like curly quotes (`'` → `'`), en/em dashes (`–` → `-`), wave dashes (`〜` → `~`), or typographic symbols (`©` → `(`). These are common in manga titles and require a supplemental fold map.

The `FoldUnicode` function applies NFKC first, then the supplemental map for ~25 characters that NFKC misses. `NFKC()` is also exported for callers that need raw NFKC without the supplemental fold.

The previous manual `unicodeFold` map and `init()` function are removed entirely.

### Alt Titles JSON Functions

`NormalizeAltTitlesJSON` and `NormalizeAllTitlesJSON` handle JSON blobs containing multiple language titles. They support two formats:

- **Structured**: `{"primary":{"en":"..."},"alts":[{"ja":"..."}]}`
- **Flat**: `{"en":"...","ja":"..."}`

`NormalizeAltTitlesJSON` applies full normalization (NFKC → custom steps → case fold) — use for comparison.
`NormalizeAllTitlesJSON` applies NFKC only, preserving case and words — use for storage boundaries.

The legacy name `FoldAltTitlesJSON` is retained as a deprecated alias for `NormalizeAllTitlesJSON`.

### Result Validation

The `queryMatchesAnyTitle` function in `commands/lookup.go` verifies MangaDex results match the user's query using three strategies:

1. Normal substring match
2. Space-removed substring match (handles apostrophe-to-space folding)
3. Apostrophe-stripped match (handles `"Reader's"` vs `"Readers"`)

### Configuration

All rules are configurable via `NormalizationConfig`:

```go
type NormalizationConfig struct {
    Separators    []string // Characters folded to spaces
    NoisePatterns []string // Regex patterns stripped from titles
    StopWords     []string // Words removed during normalisation
}
```

NFKC and Unicode case fold are provided by `golang.org/x/text` and are not configurable — they follow the Unicode standard.

Calling `New()` with a zero-value config applies all defaults automatically.

## Pipeline Diagram

```mermaid
flowchart TD
    A[Raw Title] --> B[NFKC Normalize]
    B --> B2[Supplemental Fold]
    B2 --> C[Strip File Extension]
    C --> D[Fold Separators to Spaces]
    D --> E[Remove Noise Patterns]
    E --> F[Trim & Collapse Whitespace]
    F --> G[Remove Stop Words]
    G --> H[Unicode Case Fold]
    H --> I[Canonical Form]

    subgraph "NFKC Normalization"
        B1a['Fullwidth → ASCII']
        B1b['Ligatures → expanded']
        B1c['Compatibility variants → canonical']
    end

    subgraph "Supplemental Fold"
        B2a['Curly quotes → ASCII']
        B2b['En/em dashes → hyphen']
        B2c['Wave dash → tilde']
        B2d['© ® ° × ÷ → ASCII']
    end

    subgraph "Noise Patterns"
        E1('bracketed tags')
        E2('parenthesised notes')
        E3('Vol. / Ch. / Part markers')
    end

    subgraph "Separators"
        D1['. _ - : ; , ! ? ~ etc.']
    end

    style A fill:#f9f,stroke:#333
    style I fill:#9f9,stroke:#333
```
