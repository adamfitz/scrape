# fuzzy-matching Specification

## Purpose

Provide tiered approximate title matching that handles grammar variations, spelling differences, word reordering, and minor character-level differences — ensuring all data operations (batch lookup, single lookup, API validation, deduplication) are idempotent regardless of how a title is formatted.

Fuzzy matching complements the normalisation pipeline. Normalisation handles **systematic** variations (Unicode, punctuation, case, separators, noise). Fuzzy matching handles **non-systematic** variations (contractions, typos, word order, missing/extra words).

## Architecture

Fuzzy matching spans three pillars:

| Pillar | Component | Responsibility |
|--------|-----------|---------------|
| **Normalize** | `GrammarRules` map, `Similarity()`, `Match()`, `BestMatch()` | Pure text transformation and scoring — no I/O |
| **Data** | `IndexTitle()`, `AllMedia()` | Raw storage — no normalisation logic |
| **Pipeline** | `fuzzyScan()`, `FuzzyLookup()`, `LinkQuery()`, enhanced `queryMatchesAnyTitle()` | Orchestration — the only place normalize + data meet |

## Requirements

### Requirement: Grammar expansion (normalize pillar)

The normaliser SHALL expand common English contractions and misspellings during the full normalisation pass (`Normalize()`), between NFKC/supplemental fold and separator folding. This ensures "Dont" and "Don't" both normalise to the same canonical form.

The expansion rules SHALL be stored in an extensible, package-level `GrammarRules` map. Callers MAY modify the map at runtime to add, remove, or change rules.

The `expandGrammar` function SHALL be **case-insensitive** and SHALL **strip apostrophes** (ASCII `'`, curly `'`/`\u2018`/`\u2019`, modifier letter `\u02BC`) before looking up words in the `GrammarRules` map. This ensures that "Dont", "dont", "Don't", and "don\u2019t" all match the same rule.

#### Default rules

| Contraction | Expansion |
|-------------|-----------|
| `dont` | `do not` |
| `doesnt` | `does not` |
| `didnt` | `did not` |
| `cant` | `can not` |
| `wont` | `will not` |
| `isnt` | `is not` |
| `wasnt` | `was not` |
| `arent` | `are not` |
| `werent` | `were not` |
| `hasnt` | `has not` |
| `havent` | `have not` |
| `hadnt` | `had not` |
| `wouldnt` | `would not` |
| `couldnt` | `could not` |
| `shouldnt` | `should not` |
| `mustnt` | `must not` |
| `ive` | `i have` |
| `youve` | `you have` |
| `weve` | `we have` |
| `theyve` | `they have` |
| `im` | `i am` |
| `youre` | `you are` |
| `were` | `we are` |
| `theyre` | `they are` |
| `hes` | `he is` |
| `shes` | `she is` |
| `thats` | `that is` |
| `whats` | `what is` |
| `whos` | `who is` |
| `wheres` | `where is` |
| `hows` | `how is` |
| `its` | `it is` |
| `lets` | `let us` |

#### Scenario: Contraction match

- GIVEN the database stores `"You Like Me, Don't You?"` (with apostrophe)
- WHEN a query uses `"You Like Me, Dont You"` (without apostrophe)
- THEN grammar expansion SHALL expand both `Don't` and `Dont` to `do not`
- AND both SHALL normalise to the same canonical form
- AND the query SHALL match via exact title_index (tier 1) — no fuzzy scan needed

#### Scenario: Apostrophe variant match

- GIVEN the database stores `"You Like Me, Don\u2019t You"` (curly apostrophe)
- WHEN a query uses `"You Like Me, Don't You"` (ASCII apostrophe)
- THEN `expandGrammar` SHALL strip both apostrophe variants and expand to `do not`
- AND both SHALL normalise to the same canonical form

#### Scenario: Extensibility

- GIVEN a caller adds `normalize.GrammarRules["gonna"] = "going to"`
- WHEN a query contains `"gonna"`
- THEN the normaliser SHALL expand it to `"going to"`

#### Scenario: Rule removal

- GIVEN a caller deletes the `"its"` → `"it is"` rule
- WHEN a query contains `"its"`
- THEN the normaliser SHALL NOT expand it
- AND `"its"` SHALL remain as-is in the canonical form

### Requirement: Fuzzy scoring functions (normalize pillar)

The normaliser SHALL expose pure functions for computing string similarity. These functions operate on strings only — no database, no API, no I/O.

#### Exported functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `Similarity` | `func Similarity(a, b string) float64` | Returns 0.0–1.0 similarity score. Normalises both inputs via `FoldUnicode + strings.ToLower` before scoring. Returns 0.0 for empty inputs. |
| `Match` | `func Match(a, b string, threshold float64) bool` | Returns `Similarity(a, b) >= threshold`. |
| `BestMatch` | `func BestMatch(query string, candidates []string, threshold float64) (int, float64, bool)` | Returns index, score, and ok for the closest match in candidates above threshold. |

#### Internal scoring strategy

`Similarity` applies a waterfall of strategies:

1. **Exact match** — normalised forms are identical → 1.0
2. **Substring containment** — one normalised form contains the other → 0.95
3. **Token Jaccard** — word-level set similarity (handles word reordering, missing/extra words)
4. **Levenshtein** — character-level edit distance (handles typos, character substitutions)
5. **Result** — `max(tokenJaccard, levenshteinSimilarity)`

Token Jaccard handles word reordering ("You Dont You" ≈ "Dont You You"). Levenshtein handles typos ("Yuusha" ≈ "Yusha"). Using max means either strategy can match — correct for title matching where the failure mode differs per case.

#### Scenario: Word reordering

- GIVEN query `"You Dont You"` and candidate `"Dont You You"`
- WHEN `Similarity` is called
- THEN the score SHALL be >= 0.85 (token Jaccard detects same word set)

#### Scenario: Typo tolerance

- GIVEN query `"Yuusha Party"` and candidate `"Yusha Party"`
- WHEN `Similarity` is called
- THEN the score SHALL be >= 0.85 (Levenshtein detects single character difference)

#### Scenario: False positive rejection

- GIVEN query `"One Piece"` and candidate `"Naruto"`
- WHEN `Similarity` is called
- THEN the score SHALL be < 0.5

#### Scenario: BestMatch selection

- GIVEN candidates `["One Piece", "One Punch Man", "One Piece Film"]` and query `"One Piece"`
- WHEN `BestMatch` is called with threshold 0.85
- THEN index 0 SHALL be returned with score 1.0

### Requirement: Fuzzy API result validation (pipeline pillar)

The `queryMatchesAnyTitle` function SHALL use fuzzy similarity as a 4th strategy after the existing 3 strategies (substring, space-removed, apostrophe-stripped). If all 3 exact strategies fail, the function SHALL compute `Similarity(normalizedQuery, normalized)` and accept the result if the score >= 0.85.

#### Scenario: API result with minor title difference

- GIVEN the query `"You Like Me, Dont You"`
- WHEN MangaDex returns `"You Like Me, Don't You?"` (with question mark)
- AND all 3 exact strategies fail to match
- THEN fuzzy similarity SHALL accept the result (grammar expansion makes both normalise to `"you like me do not you"`)

#### Scenario: API result rejected despite fuzzy

- GIVEN the query `"Completely Different Title"`
- WHEN MangaDex returns `"Something Unrelated"`
- AND fuzzy similarity score < 0.85
- THEN the result SHALL be rejected

### Requirement: Fuzzy local lookup (pipeline pillar)

The pipeline SHALL provide a `FuzzyLookup(mediaType, query, threshold)` method that extends `Lookup` with fuzzy fallback:

1. **Tier 1: Exact match** — O(1) via title_index (existing `Lookup`)
2. **Tier 2: Fuzzy scan** — O(N) via `AllMedia()` + `Similarity()` scoring against normalised forms
3. If single fuzzy match above threshold: auto-link via `LinkQuery` and return as `SourceFuzzy`
4. If multiple fuzzy matches above threshold: return them as `FuzzyCandidates` for disambiguation
5. If no fuzzy matches: return not found

#### Scenario: Exact match found

- GIVEN `"One Piece"` exists in the database
- WHEN `FuzzyLookup` is called with `"One Piece"`
- THEN tier 1 SHALL match
- AND no fuzzy scan SHALL be performed

#### Scenario: Fuzzy match found

- GIVEN the database stores `"You're Under My Skin!"` (with apostrophe)
- WHEN `FuzzyLookup` is called with `"Youre under my Skin"` (without apostrophe)
- THEN tier 1 SHALL miss (normalised forms differ)
- AND tier 2 SHALL find the match via fuzzy similarity

#### Scenario: Multiple fuzzy matches

- GIVEN the database stores `"Yuusha Party"` and `"Yuusha Pardy"`
- WHEN `FuzzyLookup` is called with `"Yuusha Partie"` (threshold 0.70)
- THEN tier 1 SHALL miss (no exact match)
- AND tier 2 SHALL find both matches above threshold
- AND both SHALL be returned as `FuzzyCandidates` for user disambiguation

### Requirement: Query linking for idempotency (pipeline pillar)

After a fuzzy match is confirmed (by auto-selection or user disambiguation), the pipeline SHALL create a secondary title_index entry linking the user's original query to the matched media record. This ensures future exact-match lookups for this query succeed.

The pipeline SHALL expose a `LinkQuery(mediaType, mediaID, originalQuery)` method that:
1. Normalises the original query via `MustNormalize()`
2. Calls `db.IndexTitle(mediaType, mediaID, normalised, originalQuery)`
3. Uses `INSERT OR REPLACE` (idempotent — safe to call multiple times)

#### Scenario: First run (fuzzy match + link)

- GIVEN the database has no entry for `"You Like Me, Dont You"`
- WHEN batch processes this title and fuzzy-matches it to `"You Like Me, Don't You?"` (media_id=X)
- THEN `LinkQuery` SHALL add index entry: normalised(`"You Like Me, Dont You"`) → X
- AND the result SHALL be `[fuzzy] You Like Me, Don't You?`

#### Scenario: Second run (exact match via link)

- GIVEN the LinkQuery entry from the first run exists
- WHEN batch processes the same title again
- THEN tier 1 exact match SHALL find it immediately
- AND no fuzzy scan SHALL be performed
- AND no API call SHALL be made
- AND the result SHALL be `[db] You Like Me, Don't You?`

#### Scenario: Idempotency across normalisation variants

- GIVEN `"You Like Me, Dont You"` was linked in a previous run
- WHEN a query uses `"You Like Me, Don't You"` (with apostrophe)
- THEN grammar expansion SHALL normalise both to `"you like me do not you"`
- AND tier 1 exact match SHALL succeed

### Requirement: Batch fuzzy disambiguation

When `BatchLookupStream` encounters fuzzy candidates for a title (from local scan or API results), it SHALL handle them as follows:

- **Single fuzzy match** (score >= 0.85): auto-link via `LinkQuery` and emit `LookupResult{Media, Source: SourceFuzzy}`
- **Multiple fuzzy matches**: return a result with `Candidates` populated. The batch command SHALL pause processing for that title and prompt the user to select the correct entry.

After the user selects a candidate, the batch command SHALL:
1. Ingest the selected candidate via `Ingest()` (if from API) or use the existing record (if from local fuzzy scan)
2. Link the user's query via `LinkQuery()`
3. Report the result and continue to the next title

#### Scenario: Batch with fuzzy disambiguation

- GIVEN the database stores two titles similar to `"Yuusha Party"` (e.g., `"Yuusha Pardy"`, `"Yuusha Pardee"`)
- WHEN batch processes this title and fuzzy scan finds multiple matches
- THEN the batch command SHALL display the candidates
- AND wait for user input
- AND after selection, link the result via `LinkQuery()`
- AND continue processing

#### Scenario: Batch second run — no disambiguation

- GIVEN the batch linked all fuzzy-matched titles in a previous run
- WHEN batch processes the same file again
- THEN all titles SHALL be found via exact match (tier 1)
- AND no disambiguation prompts SHALL appear
- AND the batch SHALL complete non-interactively

### Requirement: Batch source indicators

Each batch result line SHALL indicate the matching strategy used:

| Indicator | Meaning |
|-----------|---------|
| `[db]` | Exact match via title_index (tier 1) |
| `[fuzzy]` | Single fuzzy match — auto-linked via LinkQuery (tier 2) |
| `[fuzzy] [fuzzy multiple]` | Multiple fuzzy candidates — requires disambiguation via `scrape lookup` |
| `[api]` | Match via MangaDex API |

### Requirement: Configurable threshold

The fuzzy matching threshold SHALL default to 0.85. This is conservative enough that false positives are extremely unlikely for distinct manga titles, while catching all real-world spelling/grammar variations.

The threshold is set at the pipeline level and applies uniformly to all fuzzy operations (local scan, API validation, batch disambiguation).

### Requirement: Extensibility

The fuzzy matching system SHALL be extensible at multiple levels:

1. **Grammar rules** — add/remove/modify entries in `normalize.GrammarRules`
2. **Similarity threshold** — adjust the pipeline-level threshold constant
3. **Scoring algorithms** — the `Similarity` function uses composable internal strategies (token Jaccard, Levenshtein) that can be reweighted or extended
4. **Pipeline integration** — new matching strategies can be added to `queryMatchesAnyTitle` and `fuzzyScan` without changing the normalize or data pillars

### Requirement: Performance

- **Fuzzy scan** for a single title against 700 records SHALL complete in < 200ms
- **Grammar expansion** SHALL add < 1ms per title to the normalisation pass
- **Batch with fuzzy** SHALL process 700 titles in < 60 seconds (including fuzzy scans for misses)
- **Second run** (all exact matches) SHALL be as fast as the current non-fuzzy batch

### Requirement: Zero new dependencies

Fuzzy scoring functions SHALL be implemented using only the Go standard library. No external fuzzy matching libraries are permitted. The implementation uses `[]rune`-based Levenshtein distance and `strings.Fields`-based token Jaccard similarity.

## Data Flow

```
┌─────────────────────────────────────────────────────────┐
│                    NORMALIZE PILLAR                       │
│                                                           │
│  GrammarRules map (extensible)                           │
│       │                                                   │
│       ▼                                                   │
│  Normalize() ──► FoldUnicode → grammar expand →          │
│                  separator fold → noise → stop words →    │
│                  case fold                                │
│                                                           │
│  Similarity(), BestMatch(), Match()                      │
│  (fuzzy scoring: token Jaccard + Levenshtein)            │
└─────────────────────────────────────────────────────────┘
         │                              │
         ▼                              ▼
┌────────────────────────┐  ┌────────────────────────────┐
│    DATA PILLAR          │  │     PIPELINE PILLAR         │
│                         │  │                              │
│  title_index            │  │  Lookup()     (exact O(1))  │
│    ├─ IndexTitle()      │◄─│  FuzzyLookup() (fuzzy O(N))│
│    ├─ IndexTitles()     │  │  BatchLookup()              │
│    └─ QueryTitleIndex() │  │  BatchLookupStream()        │
│                         │  │  LinkQuery()                │
│  AllMedia()             │  │  queryMatchesAnyTitle()     │
│  UpsertMedia()          │  │    └─ 4th strategy: fuzzy   │
└────────────────────────┘  └────────────────────────────┘
                                      │
                                      ▼
                             ┌────────────────────────┐
                             │    COMMAND LAYER        │
                             │                         │
                             │  batch.go               │
                             │    ├─ handleFuzzyDisamb│
                             │    └─ handleAPIDisambig│
                             │  lookup.go              │
                             │    └─ handleDisambig   │
                             └────────────────────────┘
```

## Idempotency Guarantee

After the first successful batch run, subsequent runs with the same file find all titles via exact match (tier 1) — no fuzzy scanning, no API calls, no user interaction.

The guarantee rests on four properties:
1. **Deterministic normalisation** — same input always produces the same canonical form
2. **Grammar expansion** — same contractions always expand to the same forms (case-insensitive, apostrophe-stripping)
3. **LinkQuery** — secondary index entries ensure future exact-match lookups succeed
4. **Grammar unification** — "Dont", "Don't", and "don\u2019t" all normalise to "do not" via the same GrammarRules entry

## See also

- `name-normalization/spec.md` — the normalisation pipeline that handles systematic variants
- `batch-lookup/spec.md` — batch processing with fuzzy disambiguation
- `single-lookup/spec.md` — single title lookup with fuzzy fallback
- `architecture-analysis.md` — the three-pillar structural context
