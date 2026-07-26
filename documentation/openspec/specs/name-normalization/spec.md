# name-normalization Specification

## Purpose

Transform raw titles into deterministic canonical forms for consistent comparison and lookup. Normalisation is applied at two levels: **Unicode folding** (at storage boundaries) and **full normalisation** (at comparison time). Together they ensure titles from any source — user input, file imports, or external APIs — are matched consistently regardless of Unicode variant differences.

All normalisation is invoked exclusively through the pipeline component (`internal/pipeline/`). Commands import only `pipeline/` and `database/` — they do not call normaliser functions directly. See `documentation/architecture-analysis.md` for the full structural context.

This specification applies to all title-bearing tables: `manga`, `anime`, `lightnovels`, `webnovels`, `webtoons`. The normaliser is media-type agnostic — the pipeline passes the appropriate table type to each operation.

## Requirements

### Requirement: NFKC normalization (storage boundary)

The pipeline SHALL apply Unicode NFKC normalization at storage boundaries for all title-bearing tables (`manga`, `anime`, `lightnovels`, `webnovels`, `webtoons`), followed by a supplemental fold for characters that NFKC does not handle.

NFKC decomposes characters to their canonical form and maps compatibility characters to their equivalents (fullwidth → ASCII, ligatures → expanded, etc.). This covers thousands of characters — far more than the previous manual `unicodeFold` map.

However, NFKC does NOT handle **semantically similar characters** like curly quotes, en/em dashes, wave dashes, or typographic symbols. A supplemental fold map handles these ~25 additional characters after the NFKC pass.

Non-Latin scripts (Hangul, Kanji, Kana, CJK) SHALL NOT be modified — they are already in their canonical form.

#### Scenario: Apostrophe variant match

- GIVEN the database stores `"Hero's Party"` (ASCII apostrophe)
- WHEN a query uses `"Hero\u2019s Party"` (right single quotation mark)
- THEN NFKC + supplemental fold SHALL map `\u2019` → `'` at storage time
- AND the titles SHALL match via normalized comparison

#### Scenario: Tilde variant match

- GIVEN the database stores `"Title~ Subtitle"` (ASCII tilde)
- WHEN a query uses `"Title\uFF5E Subtitle"` (fullwidth tilde)
- THEN NFKC SHALL map `\uFF5E` → `~` (fullwidth is a compatibility equivalent)
- AND both SHALL normalize to the same canonical form

#### Scenario: Em dash fold

- GIVEN the database stores `"Title - Subtitle"` (hyphen-minus)
- WHEN a query uses `"Title \u2014 Subtitle"` (em dash)
- THEN supplemental fold SHALL map `\u2014` → `-` at storage time
- AND both SHALL normalize to the same canonical form

#### Scenario: Hangul preservation

- GIVEN the input `"외모지상주의"`
- WHEN NFKC normalisation is applied
- THEN the Hangul characters SHALL NOT be modified

#### Scenario: CJK preservation

- GIVEN the input `"鬼滅の刃"`
- WHEN NFKC normalisation is applied
- THEN the Kanji/Kana characters SHALL NOT be modified

### Requirement: Full normalization (comparison boundary)

The pipeline SHALL apply the following transformations in order to produce the canonical form used for matching:

1. **NFKC normalize + supplemental fold** — Unicode NFKC via `golang.org/x/text/unicode/norm`, then a supplemental fold map for ~25 characters NFKC misses (curly quotes, en/em dashes, wave dashes, typographic symbols). This replaces the previous manual `unicodeFold` map.
2. **Grammar expansion** — expand common English contractions and misspellings using an extensible `GrammarRules` map (e.g., `dont` → `do not`, `youre` → `you are`). Applied after fold, before separator folding. Ensures contracted and expanded forms normalise identically.
3. **File extension strip** — remove media file extensions (custom regex)
4. **Separator fold** — convert punctuation to spaces (custom regex, expanded character set)
5. **Noise removal** — remove bracketed tags, parenthesized notes, volume/chapter/part markers, edition markers (custom regex)
6. **Multi-space collapse** — collapse consecutive spaces
7. **Stop word removal** — remove configurable stop words
8. **Unicode case fold** — case-insensitive folding (`golang.org/x/text/cases`). Replaces `strings.ToLower` to handle `ß → ss`, Kelvin `K → k`, and other locale-aware case mappings.

#### Scenario: NFKC fullwidth fold

- GIVEN the input `"Ｍｏｎｓｔｅｒ ＃８"`
- WHEN NFKC normalisation is applied
- THEN the result SHALL be `"Monster #8"` (fullwidth → ASCII via NFKC compatibility decomposition)

#### Scenario: NFKC ligature expansion

- GIVEN a title containing `"ﬁ"` (U+FB01, Latin small ligature fi)
- WHEN NFKC normalisation is applied
- THEN the result SHALL contain `"fi"` (expanded)

#### Scenario: Unicode case fold

- GIVEN the input `"straße"`
- WHEN Unicode case folding is applied
- THEN the result SHALL match the fold of `"STRASSE"` (`ß → ss`)

### Requirement: Separator folding

The normalizer SHALL convert common word separators and punctuation into spaces:

| Character | Description |
|-----------|-------------|
| `.` | Dot |
| `_` | Underscore |
| `-` | Hyphen |
| `:` | Colon |
| `;` | Semicolon |
| `,` | Comma |
| `!` | Exclamation |
| `?` | Question mark |
| `'` | Curly quote |
| `'` | Curly quote |
| `"` | Double quote |
| `~` | Tilde |
| `～` | Fullwidth tilde |
| `〜` | Wave dash |

#### Scenario: Colon-separated title

- GIVEN the input `"After Being Called a Hero: The Unrivaled Man Starts a Family"`
- WHEN normalized
- THEN the result SHALL be `"after being called a hero the unrivaled man starts a family"`

#### Scenario: Dot-separated title

- GIVEN the input `"Chainsaw.Man"`
- WHEN normalized
- THEN the result SHALL be `"chainsaw man"`

### Requirement: File extension stripping

The normalizer SHALL remove common media file extensions (`.cbz`, `.cbr`, `.mkv`, `.epub`, etc.) before processing.

#### Scenario: CBZ extension removal

- GIVEN the input `"One.Piece.Ch.1090.cbz"`
- WHEN normalized
- THEN the result SHALL be `"one piece"`

### Requirement: Noise removal

The normalizer SHALL remove bracketed tags `[...]`, parenthesized notes `(...)`, and volume/chapter/part markers.

#### Scenario: Scan group tag removal

- GIVEN the input `"[ScanGroup] Bloom Into You (Digital)"`
- WHEN normalized
- THEN the result SHALL be `"bloom into you"`

### Requirement: Stop word removal

The normalizer SHALL remove configurable English stop words (`the`, `a`, `an`, `and`, `of`) by default.

#### Scenario: Leading stop word removal

- GIVEN the input `"The Manga Name"`
- WHEN normalized
- THEN the result SHALL be `"manga name"`

### Requirement: Multi-space collapsing

The normalizer SHALL collapse multiple consecutive spaces into a single space.

#### Scenario: Multiple spaces

- GIVEN the input `"Title:   Subtitle"`
- WHEN normalized
- THEN the result SHALL be `"title subtitle"`

### Requirement: Unicode case folding

The pipeline SHALL apply Unicode case folding to produce a case-insensitive canonical form. This replaces `strings.ToLower` to handle locale-aware case mappings (`ß → ss`, Kelvin `K → k`, etc.). The `golang.org/x/text/cases` package provides the standard implementation.

#### Scenario: Case-insensitive match

- GIVEN the input `"One Piece"`
- WHEN normalised
- THEN the result SHALL be `"one piece"`

#### Scenario: Mixed case match

- GIVEN the input `"NARUTO"`
- WHEN normalised
- THEN the result SHALL be `"naruto"`

### Requirement: Non-Latin script preservation

The normalizer SHALL preserve non-Latin characters (Japanese kanji/kana, Korean hangul, Chinese characters) without modification.

#### Scenario: Japanese kanji preservation

- GIVEN the input `"鬼滅の刃"`
- WHEN normalized
- THEN the result SHALL be `"鬼滅の刃"`

#### Scenario: Korean hangul preservation

- GIVEN the input `"외모지상주의"`
- WHEN normalized
- THEN the result SHALL be `"외모지상주의"`

### Requirement: Grammar expansion

The normaliser SHALL expand common English contractions and misspellings during the normalisation pass, between NFKC/supplemental fold and separator folding. The expansion rules SHALL be stored in an extensible, package-level `GrammarRules` map.

The `expandGrammar` function SHALL be **case-insensitive** and SHALL **strip apostrophes** (ASCII `'`, curly `\u2018`/`\u2019`, modifier letter `\u02BC`) before looking up words. This ensures "Dont", "Don't", and "don\u2019t" all match the same rule.

#### Scenario: Contraction expansion

- GIVEN the input `"You Like Me, Dont You"`
- WHEN normalized
- THEN the result SHALL be `"you like me do not you"` (dont expanded to do not)

#### Scenario: Apostrophe contraction

- GIVEN the input `"You're Under My Skin"`
- WHEN normalized
- THEN the result SHALL be `"you are under my skin"` (expandGrammar strips apostrophe, matches "youre" → "you are")

#### Scenario: Apostrophe variant unification

- GIVEN the input `"You Like Me, Don't You"` (ASCII apostrophe)
- WHEN normalized
- THEN `expandGrammar` SHALL strip the apostrophe, lowercase, and match `"dont"` → `"do not"`
- AND the result SHALL be `"you like me do not you"`

#### Scenario: Extensibility

- GIVEN a caller adds `normalize.GrammarRules["gonna"] = "going to"`
- WHEN a query contains `"gonna"`
- THEN the normaliser SHALL expand it to `"going to"`

### Requirement: Determinism

The normalizer SHALL produce identical output for identical input across multiple calls.

### Requirement: Original preservation

The normalizer SHALL NOT modify the original input string.

### Requirement: Reusable alt-title JSON normalization

The pipeline SHALL use the following exported functions for normalising alt_title JSON blobs:

- `NormalizeAltTitlesJSON(raw string) string` — full normalization of all title values (NFKC → custom → case fold) for comparison
- `NormalizeAllTitlesJSON(raw string) string` — NFKC normalization only, preserving case and words (for storage)

Both functions SHALL handle structured format (`{"primary":{...},"alts":[{...}]}`) and flat format (`{"en":"...","ja":"..."}`).

## Configuration

```go
type NormalizationConfig struct {
    Separators    []string  // default: [".", "_", "-", ":", ";", ",", "!", "?", "'", "'", "\"", "~", "～", "〜"]
    NoisePatterns []string  // default: bracket/paren/vol/ch/edition patterns
    StopWords     []string  // default: ["the", "a", "an", "and", "of"]
}
```

NFKC and Unicode case fold are provided by `golang.org/x/text` and are not configurable — they follow the Unicode standard.

Grammar expansion rules are stored in the package-level `GrammarRules` map (separate from `NormalizationConfig`). This allows runtime modification without reconstructing the normaliser.

## Exported Functions

| Function | Purpose | Use at |
|----------|---------|--------|
| `New(config) *Normalizer` | Create normalizer instance | Any |
| `(*Normalizer).Normalize(name) (string, error)` | Full canonical form (NFKC + supplemental → grammar → custom → case fold) | Comparison boundary |
| `(*Normalizer).MustNormalize(name) string` | Full canonical form (panics on error) | Comparison boundary |
| `FoldUnicode(s string) string` | NFKC + supplemental fold (preserves case/words) | Storage boundary |
| `NFKC(s string) string` | Raw NFKC only (no supplemental fold) | When NFKC alone is needed |
| `NormalizeAltTitlesJSON(raw string) string` | Full normalize all values in alt_title JSON | Comparison boundary |
| `NormalizeAllTitlesJSON(raw string) string` | NFKC + supplemental fold all values in alt_title JSON (preserves case/words) | Storage boundary |
| `Similarity(a, b string) float64` | 0.0–1.0 string similarity score (normalised via FoldUnicode + lowercase) | Fuzzy matching |
| `Match(a, b string, threshold float64) bool` | Threshold-gated similarity check | Fuzzy matching |
| `BestMatch(query string, candidates []string, threshold float64) (int, float64, bool)` | Find best match in candidate list | Fuzzy matching |

**Note**: The manual `unicodeFold` map is removed entirely. `FoldUnicode` now applies NFKC + supplemental fold. `FoldAltTitlesJSON` is removed — use `NormalizeAllTitlesJSON` for storage boundary JSON normalization.

## Implementation Notes

- NFKC runs first to decompose compatibility characters, then the supplemental fold handles characters NFKC misses (curly quotes, dashes, typographic symbols)
- Unicode case fold runs last as the final canonicalisation step
- The pipeline is media-agnostic: works for manga, anime, light novels, web novels, webtoons
- The pipeline component is the single entry point — commands never call normaliser functions directly
- Domain-specific steps (extensions, separators, noise, stop words) remain as custom layers between fold and case fold
- The normaliser is immutable after construction — safe for concurrent use by multiple goroutines
- See `documentation/known-issues.md` for current shortcomings and improvement roadmap
- See `documentation/architecture-analysis.md` for the three-pillar structural context
