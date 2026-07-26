# Normalisation Known Issues

Consolidated analysis of shortcomings in `internal/normalize/normalize.go`, the name-normalization spec, and the technical documentation. Organised by severity.

---

## Table of Contents

- [Critical: Breaks Idempotency / Determinism](#critical-breaks-idempotency--determinism)
  - [#1 — JSON Map Key Ordering](#1-json-map-key-ordering--non-deterministic-output)
  - [#2 — FoldAltTitlesJSON vs NormalizeAltTitlesJSON](#2-foldaltitlesjson-vs-normalizealtitlesjson--two-fork-paths)
  - [#3 — Unicode Fold Table Applied at Write, Not Full Normalise](#3-unicode-fold-table-applied-at-write-not-full-normalise)
- [Moderate: Affects Accuracy / Edge Cases](#moderate-affects-accuracy--edge-cases)
  - [#4 — Ampersand Not in Fold Table](#4--ampersand-not-in-fold-table-or-separator-list)
  - [#5 — Pipe Not in Separator List](#5--pipe-not-in-separator-list)
  - [#6 — Edition/Version Markers Not Stripped](#6-edition--version-markers-not-stripped)
  - [#7 — Bare Year Numbers Not Stripped](#7-bare-year-numbers-not-stripped)
  - [#8 — Aggressive Noise Removal](#8-aggressive-noise-removal--legitimate-content-stripped)
  - [#9 — Incomplete Unicode Fold Table](#9-incomplete-unicode-fold-table)
  - [#10 — No Unicode Case Folding](#10-no-unicode-case-folding)
  - [#11 — Missing Separators](#11-missing-separators)
  - [#12 — Volume/Chapter Regex Limitations](#12-volumechapter-regex-doesnt-handle-roman-numerals)
  - [#13 — Part Number Stripping Removes Distinction](#13-part-number-stripping-removes-distinction-between-works)
- [Minor: Fuzzy Matching & Test Coverage](#minor-fuzzy-matching--test-coverage)
  - [#14 — Fuzzy Threshold May Need Tuning](#14-fuzzy-matching-threshold-may-need-tuning)
  - [#15 — Grammar Expansion Limited to English](#15-grammar-expansion-limited-to-english-contractions)
  - [#16 — Fuzzy Scan O(N)](#16-fuzzy-scan-on-for-large-databases)
  - [#17 — No Idempotency Tests for JSON](#17-no-idempotency-tests-for-json-functions)
  - [#18 — No Transitivity Test](#18-no-transitivity-test-for-normalize)
  - [#19 — Weak AccentPreserved Test](#19-accentpreserved-test-is-weak)
  - [#20 — No NFKC Equivalence Test](#20-no-nfkc-equivalence-test)
  - [#21 — No Cross-Script Equivalence Test](#21-no-cross-script-equivalence-test)
  - [#22 — test_list.txt Duplicate Entry](#22-test_listtxt-duplicate-entry)
  - [#23 — No test_list.txt Full Run Against Expected](#23-no-test_listtxt-full-run-against-expected)
- [Unicode Standards Reference](#unicode-standards-reference)
- [Existing Libraries](#existing-libraries)

---

## Critical: Breaks Idempotency / Determinism

### 1. ~~JSON Map Key Ordering — Non-Deterministic Output~~ — RESOLVED

**Resolution**: Both `NormalizeAllTitlesJSON` and `NormalizeAltTitlesJSON` now use sorted maps via `marshalMapSorted()`. Map keys are sorted before JSON serialization, producing deterministic output. The `FoldAltTitlesJSON` function has been removed.

**Verification**: The `marshalMapSorted` helper sorts keys alphabetically before writing JSON.

### 2. ~~`NormalizeAllTitles` Only Normalises the `manga` Table~~ — RESOLVED

**Resolution**: `NormalizeAllTitles` now lives in `internal/pipeline/` and iterates over all 5 media types via `database.AllMediaTypes()`. Each table's titles are re-normalised via `FoldUnicode` and `NormalizeAllTitlesJSON`. The pipeline is the single entry point for this operation.

### 3. ~~No Storage-Boundary Normalisation for Non-Manga Tables~~ — RESOLVED

**Resolution**: The pipeline's `Ingest` function applies `FoldUnicode` to titles and `NormalizeAllTitlesJSON` to alt_title JSON before calling `db.UpsertMedia()`. This applies to all media types — the pipeline is media-agnostic. The data layer (`internal/database/`) stores raw data without normalisation; the pipeline is responsible for all normalisation at storage boundaries.

---

## Moderate: Affects Accuracy / Edge Cases

### 4. ~~`&` (Ampersand) Not in Fold Table or Separator List~~ — RESOLVED

**Resolution**: `&` has been added to the default separator list. It is now folded to a space during normalisation, so `"Dungeons & Artifacts"` normalises to the same canonical form as `"Dungeons and Artifacts"`.

### 5. ~~`|` (Pipe) Not in Separator List~~ — RESOLVED

**Resolution**: `|` has been added to the default separator list. It is now folded to a space during normalisation.

### 6. ~~Edition / Version Markers Not Stripped~~ — RESOLVED

**Resolution**: Noise patterns have been added for non-parenthesized edition markers: `Perfect Edition`, `Omnibus Edition`, `2-in-1 Edition`. Parenthesized markers like `(Digital)` and `(Pre-Serialization)` were already stripped by the existing `\(.*?\)` pattern.

### 7. ~~Bare Year Numbers Not Stripped~~ — RESOLVED

**Resolution**: A trailing year pattern (`\s\d{4}$`) has been added to the noise patterns. Bare 4-digit years at the end of titles are now stripped. Years in the middle of titles (followed by other words) are preserved to avoid false matches.

### 8. Aggressive Noise Removal — Legitimate Content Stripped

**File**: `normalize.go:42-43`

`\[.*?\]` and `\(.*?\)` remove ALL bracketed/parenthesised content indiscriminately:

| Input | Normalised | Content Lost |
|-------|-----------|-------------|
| `"Fate/Stay Night (Heaven's Feel)"` | `"fate stay night"` | `heavens feel` |
| `"Title [Digital] v2"` | `"title"` | `digital` |
| `"Monster #8 (Vol. 1-5)"` | `"monster 8"` | vol range |

This is a design choice but means titles with meaningful parenthetical/bracketed content lose information, potentially causing false matches between different works.

### 9. ~~Incomplete Unicode Fold Table~~ — RESOLVED

**Resolution**: NFKC normalization + supplemental fold map replaces the manual `unicodeFold` table. NFKC handles thousands of compatibility characters (fullwidth, ligatures, etc.). The supplemental fold handles ~25 characters NFKC misses (curly quotes, en/em dashes, wave dashes, typographic symbols). `FoldUnicode()` now applies NFKC + supplemental fold. The manual `unicodeFold` map and `init()` are removed.

**Remaining gap**: `&` (ampersand), `₩` (Korean won), `￥` (yen sign), `￠` (cent sign), `§` (section sign), `•` (bullet), `⁄` (fraction slash) are still not in the separator list or supplemental fold. These are handled by Issue [#4](#4--ampersand-not-in-fold-table-or-separator-list) and [#11](#11-missing-separators) as separator concerns, not fold concerns.

### 10. ~~No Unicode Case Folding~~ — RESOLVED

**Resolution**: `strings.ToLower()` replaced by `cases.Fold()` from `golang.org/x/text/cases`. Unicode case-equivalent characters are now folded: `ß` → `ss`, Kelvin `K` → `k`, `ſ` → `s`, etc.

### 11. ~~Missing Separators~~ — RESOLVED

**Resolution**: `&`, `|`, and `•` have been added to the default separator list. They are now folded to spaces during normalisation.

### 12. ~~Volume/Chapter Regex Doesn't Handle Roman Numerals~~ — RESOLVED

**Resolution**: The volume/chapter regex patterns now accept both Arabic digits and Roman numerals: `\d+|[ivxlcdm]+`. Titles like `"Manga Title Vol. III"` are now correctly stripped to `"manga title"`.

### 13. ~~Part Number Stripping Removes Distinction Between Works~~ — RESOLVED

**Resolution**: The part number stripping pattern (`\s(part|pt)\.?\s*\d+`) has been removed. Part numbers are now preserved in the canonical form, so `"Ascendance of a Bookworm - Part 01"` and `"Part 02"` produce distinct canonical forms.

### 14. ~~Fuzzy Matching Threshold May Need Tuning~~ — Implemented

**Status**: Implemented with default threshold 0.85

The fuzzy matching threshold (0.85) is a conservative default. It is configurable at the pipeline level via the `FuzzyLookup` threshold parameter. Grammar expansion now handles most contraction variants at normalise time, reducing the burden on fuzzy matching.

**Mitigation**: Monitor false positive/negative rates in production. The threshold is configurable.

### 15. ~~Grammar Expansion Limited to English Contractions~~ — Implemented

**Status**: Implemented with `GrammarRules` map (36 entries)

The grammar expansion handles English contractions and common misspellings. The `GrammarRules` map is extensible at runtime — add rules for other languages as needed. The `expandGrammar` function is case-insensitive and strips apostrophes before lookup.

**Mitigation**: Add rules for other languages via `normalize.GrammarRules[key] = value`.

### 16. ~~Fuzzy Scan O(N) for Large Databases~~ — Implemented

**Status**: Implemented

The fuzzy scan loads all media records via `AllMedia()` and computes similarity for each. For databases with thousands of records, this could be slow. Current database size (~700 records) makes this acceptable (<200ms). The title_index handles the common case (exact match) in O(1). Fuzzy scan only runs when exact match fails.

**Mitigation**: Future optimization: add a similarity column to the title_index, or use a BK-tree for O(log N) fuzzy lookup.

---

## Minor: Test Coverage Gaps

### 17. ~~No Idempotency Tests for JSON Functions~~ — RESOLVED

**Resolution**: `TestNormalizeAltTitlesJSON_Idempotent` and `TestNormalizeAllTitlesJSON_Idempotent` now verify that calling each function twice produces identical output.

### 18. ~~No Transitivity Test for Normalize~~ — RESOLVED

**Resolution**: `TestNormalize_Transitivity` now verifies `Normalize(Normalize(x)) == Normalize(x)` for multiple inputs. `TestNormalize_Idempotency` also verifies repeated calls produce identical output.

### 19. ~~Weak AccentPreserved Test~~ — RESOLVED

**Resolution**: The test now runs against multiple diverse inputs including CJK, fullwidth, grammar expansion, and apostrophe variants.

### 20. ~~No Nested Bracket / Parenthesis Tests~~ — RESOLVED

**Resolution**: `TestNormalize_NestedBrackets` tests both `[A] [B] Title` and `Title (foo (bar))` patterns. Documents that non-greedy matching leaves trailing characters from nested groups.

### 21. ~~No Test for Edition Markers~~ — RESOLVED

**Resolution**: `TestNormalize_EditionMarker_PerfectEdition`, `TestNormalize_EditionMarker_OmnibusEdition`, and `TestNormalize_EditionMarker_TwoInOne` now validate edition marker stripping.

### 22. ~~No Test for `&` or `|` Characters~~ — RESOLVED

**Resolution**: `TestNormalize_AmpersandSeparator`, `TestNormalize_PipeSeparator`, and `TestNormalize_BulletSeparator` now validate separator folding.

### 23. Duplicate Entry in test_list.txt

`Kimi.wa.Meido-sama` and `Kimi wa Meido-sama` are two different directory names for the same manga. This is valid variant data — the normalizer handles both forms correctly.

---

## Unicode Standards & Reference

### Applicable Standards

| Standard | Full Name | URL | What It Covers |
|----------|-----------|-----|----------------|
| **UTS #39** | Unicode Security Mechanisms | `unicode.org/reports/tr39/` | Confusable character detection, mixed-script detection, identifier security profiles. Defines the "skeleton" algorithm for comparing visually similar characters across scripts. |
| **UTR #15** | Unicode Normalization | `unicode.org/reports/tr15/` | NFC, NFD, NFKC, NFKD normalization forms. Defines how to canonicalize Unicode text so equivalent representations compare equal. |
| **UAX #15 (alt)** | Unicode Normalization Forms | Same as UTR #15 | Informative alias; same document. |
| **confusables.txt** | Unicode Confusable Characters | `unicode.org/Public/security/latest/confusables.txt` | Machine-readable mapping of ~1,400 confusable character pairs. Updated each Unicode version. The raw data behind TR39 confusable detection. |
| **RFC 8264** | PRECIS Framework | `rfc-editor.org/rfc/rfc8264` | IETF framework for preparing usernames/passwords. Mandates NFKC normalization + case folding + additional checks. Used in modern protocols (XMPP, SIP, etc.). |

### Key Distinctions: Current Approach vs Standards

The current normalisation in `internal/normalize/normalize.go` is a **manually curated subset** of what the Unicode standards provide. Here is how they differ:

#### 1. Unicode Normalisation (NFKC) vs Custom Fold Table

| Aspect | Current Approach | Standard (NFKC) |
|--------|-----------------|-----------------|
| **Scope** | ~90 hand-picked characters in `unicodeFold` map | Covers all compatibility equivalents in Unicode (thousands of characters) |
| **Fullwidth letters** | `Ａ`-`Ｚ`, `ａ`-`ｚ` (52 chars) | All fullwidth forms, plus halfwidth katakana, etc. |
| **Ligatures** | Not handled | `ﬁ` → `fi`, `ﬂ` → `fl`, `ﬀ` → `ff`, etc. |
| **Superscripts/subscripts** | Not handled | `²` → `2`, `₁` → `1`, etc. |
| **Mathematical symbols** | Not handled | `∑` → `Σ`, `∞` → `∞`, etc. |
| **Compatibility decomposition** | Not handled | `Ω` (Ohm sign) → `Ω` (Greek Omega), `ℬ` → `B`, etc. |
| **Hangul safety** | Preserved (manual approach) | NFKD decomposes Hangul syllables into Jamo — this is why the codebase avoids NFKD |
| **Case folding** | ASCII-only `strings.ToLower` | Unicode-aware case folding (`ß` → `ss`, Kelvin `K` → `k`, etc.) |

**Recommendation**: Use Go's `golang.org/x/text/unicode/norm` package for NFKC normalization as a baseline, then layer custom rules (noise removal, separator folding, stop words) on top. This replaces the manual fold table with a standards-compliant foundation.

#### 2. Confusable Detection (TR39) vs Custom Fold Table

| Aspect | Current Approach | Standard (TR39) |
|--------|-----------------|-----------------|
| **Cross-script confusables** | Not handled | Cyrillic `а` (U+0430) ≈ Latin `a` (U+0061), Greek `ο` (U+03BF) ≈ Latin `o`, etc. |
| **Mixed-script detection** | Not handled | Flags input mixing Latin + Cyrillic + Greek, etc. (almost always an attack or error) |
| **Skeleton algorithm** | Not implemented | Produces a canonical "visual skeleton" for any string, enabling comparison across scripts |
| **Data source** | Manual mapping | `confusables.txt` — maintained by the Unicode Consortium, ~1,400 pairs |
| **Scope** | Characters that look like ASCII | Characters that look like *each other* across all scripts |

**Note**: Cross-script confusables are less relevant for manga title matching (titles are typically in one script), but are critical if the system ever handles user-generated identifiers, URLs, or cross-language search.

#### 3. Pipeline Order: TR39 vs NFKC Interaction

The TR39 skeleton algorithm uses **NFD** decomposition, not NFKC. This matters because:

- NFKC changes some characters *before* the confusable map sees them (e.g., `ſ` long-s → `s` via NFKC)
- TR39 maps `ſ` → `f` (visually similar to `f`, not `s`)
- If you run NFKC first, the TR39 entry for `ſ` becomes unreachable dead code

**For this codebase**: NFKC is the right baseline (we want `ſ` → `s`, not `ſ` → `f`). The custom fold table is essentially a partial NFKC implementation. A full NFKC pass would subsume most of the fold table.

### Reference Data Files

| File | URL | Format | Entries |
|------|-----|--------|---------|
| `confusables.txt` | `unicode.org/Public/security/latest/confusables.txt` | Semicolon-delimited mapping | ~1,400 single-char mappings |
| `confusables.txt` (NFKC-filtered) | Generated by `namespace-guard` | Filtered for NFKC pipelines | ~613 entries |
| `DerivedNormalizationProps.txt` | `unicode.org/Public/UCD/latest/ucd/` | Unicode character properties | Defines NFKC/NFD behavior per character |

---

## Existing Libraries

### Go Libraries

| Library | URL | What It Does | Relevance |
|---------|-----|-------------|-----------|
| `golang.org/x/text/unicode/norm` | `pkg.go.dev/golang.org/x/text/unicode/norm` | Standard NFKC/NFD/NFC/NFD normalization. Part of the Go supplementary text libraries. | **Direct replacement for the manual fold table.** Handles all compatibility equivalents, fullwidth, ligatures, etc. |
| `github.com/eskriett/confusables` | `pkg.go.dev/github.com/eskriett/confusables` | TR39 confusable detection in Go. Provides `IsConfusable()` and related functions. | Cross-script confusable detection. Useful if extending beyond title matching. |
| `golang.org/x/text/cases` | `pkg.go.dev/golang.org/x/text/cases` | Unicode-aware case folding. Handles `ß` → `ss`, Kelvin `K` → `k`, Turkish `I` → `ı`, etc. | **Direct replacement for `strings.ToLower`** in the normalisation pipeline. |
| `golang.org/x/text/unicode/bidi` | Part of `golang.org/x/text` | Bidirectional text handling. | Less relevant for title matching, but useful for mixed LTR/RTL content. |

### Libraries in Other Languages (For Reference)

| Language | Library | URL | What It Does |
|----------|---------|-----|-------------|
| Rust | `unicode-security` crate | `crates.io/crates/unicode-security` | TR39 `skeleton()` function, confusable detection |
| Rust | `unicode-normalization` crate | `crates.io/crates/unicode-normalization` | NFKC/NFD/NFC normalization |
| Python | `unicodedata` (stdlib) | `docs.python.org/3/library/unicodedata.html` | `unicodedata.normalize('NFKC', s)` — built-in |
| Python | `confusables` | `pypi.org/project/confusables/` | TR39 confusable detection |
| Python | `translit` | `translit.readthedocs.io` | Confusable detection + normalization with multi-target script support |
| TypeScript | `namespace-guard` | `npmjs.com/package/namespace-guard` | NFKC + confusable maps (two versions: filtered/unfiltered), `canonicalise()`, `scan()`, `confusableDistance()` |
| TypeScript | `confusable-vision` | `github.com/paultendo/confusable-vision` | Weight data for 1,397 confusable pairs (793 not in TR39), discovered via font rendering analysis |
| ICU (C/C++/Java/etc.) | `SpoofChecker` | `unicode-org.github.io/icu-docs/` | The canonical TR39 implementation. `getSkeleton()` uses NFD per spec. |

### Recommended Go Pipeline

Based on the standards and libraries above, the ideal normalisation pipeline for this codebase would be:

```
Raw title
  → NFKC normalize          (golang.org/x/text/unicode/norm)
  → Supplemental fold        (curly quotes, dashes, wave dashes)
  → Grammar expansion        (GrammarRules map: dont → do not, etc.)
  → Strip file extensions    (custom regex, as now)
  → Fold separators          (custom regex, as now — but with expanded set)
  → Remove noise             (custom regex, as now — but with edition markers)
  → Collapse whitespace      (custom, as now)
  → Remove stop words        (custom, as now)
  → Unicode case fold        (golang.org/x/text/cases — replaces strings.ToLower)
```

NFKC replaces the ~90-entry manual `unicodeFold` map with a standards-compliant pass covering thousands of characters. Grammar expansion handles contraction/misspelling variations. Unicode case folding replaces `strings.ToLower` to handle `ß` → `ss`, Kelvin `K` → `k`, etc. The domain-specific steps (extensions, separators, noise, stop words) remain as custom layers.

**Fuzzy matching** (scored via `Similarity()` in `internal/normalize/fuzzy.go`) operates as a fallback when exact normalised matching fails — used for local DB scan, API result validation, and batch disambiguation.

---

## Summary

| # | Issue | Severity | Status |
|---|-------|----------|--------|
| 1 | JSON map key ordering | **Critical** | **Resolved** — sorted maps via `marshalMapSorted()` |
| 2 | `NormalizeAllTitles` only manga table | **Critical** | **Resolved** — pipeline normalises all 5 tables |
| 3 | No storage-boundary normalisation for non-manga | **Critical** | **Resolved** — pipeline `Ingest` normalises for all media types |
| 4 | `&` not in fold/separators | Moderate | **Resolved** — `&` added to default separator list |
| 5 | `\|` not in separators | Moderate | **Resolved** — `|` added to default separator list |
| 6 | Edition markers not stripped | Moderate | **Resolved** — `Perfect Edition`, `Omnibus Edition`, `2-in-1 Edition` added to noise patterns |
| 7 | Bare year numbers survive | Moderate | **Resolved** — trailing `\s\d{4}$` pattern strips end-of-string years |
| 8 | Aggressive noise removal | Moderate | **Open** — by design, but nested groups may leave trailing characters |
| 9 | Incomplete Unicode fold table | Moderate | **Resolved** — NFKC + supplemental fold |
| 10 | No Unicode case folding | Moderate | **Resolved** — `cases.Fold()` |
| 11 | Missing separators | Moderate | **Resolved** — `&`, `\|`, `•` added to default list |
| 12 | Roman numeral vol/ch not matched | Moderate | **Resolved** — regex now accepts `\d+\|[ivxlcdm]+` |
| 13 | Part number stripping removes distinction | Moderate | **Resolved** — part number pattern removed, numbers preserved |
| 14 | Fuzzy threshold may need tuning | Minor | **Implemented** — default 0.85, configurable via threshold parameter |
| 15 | Grammar expansion limited to English | Minor | **Implemented** — extensible `GrammarRules` map (36 rules) |
| 16 | Fuzzy scan O(N) for large databases | Minor | **Implemented** — acceptable for current ~700 records |
| 17 | No idempotency tests for JSON functions | Minor | **Resolved** — tests added for both JSON normalisation functions |
| 18 | No transitivity test for Normalize | Minor | **Resolved** — `TestNormalize_Transitivity` and `TestNormalize_Idempotency` added |
| 19 | Weak accent preservation test | Minor | **Resolved** — test expanded with diverse inputs |
| 20 | No nested bracket tests | Minor | **Resolved** — `TestNormalize_NestedBrackets` added |
| 21 | No edition marker tests | Minor | **Resolved** — tests added for `Perfect Edition`, `Omnibus Edition`, `2-in-1 Edition` |
| 22 | No `&` / `\|` tests | Minor | **Resolved** — separator tests added for `&`, `\|`, `•` |
| 23 | Duplicate in test_list.txt | Minor | **Not a bug** — two different directory name variants for the same manga, valid test data |
