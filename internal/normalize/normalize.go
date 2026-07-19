// Package normalize transforms raw names into deterministic canonical forms.
//
// Normalization applies Unicode NFKC normalization, folds separators, removes
// structural noise (bracketed tags, volume markers), strips configurable stop
// words, and applies Unicode case folding. The original name is never modified.
package normalize

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// NormalizationConfig holds configurable normalization rules.
type NormalizationConfig struct {
	Separators    []string
	NoisePatterns []string
	StopWords     []string
}

// Normalizer applies configurable normalization rules to produce canonical forms.
type Normalizer struct {
	config   NormalizationConfig
	sepRe    *regexp.Regexp
	noiseRe  *regexp.Regexp
	stopSet  map[string]bool
	caseFold cases.Caser
}

// New returns a Normalizer with the given config (defaults applied for nil slices).
func New(config NormalizationConfig) *Normalizer {
	n := &Normalizer{config: normalizeConfig(config)}
	n.caseFold = cases.Fold()
	n.compile()
	return n
}

func normalizeConfig(c NormalizationConfig) NormalizationConfig {
	if len(c.Separators) == 0 {
		c.Separators = []string{".", "_", "-", ":", ";", ",", "!", "?", "'", "'", "\"", "~", "～", "〜"}
	}
	if len(c.NoisePatterns) == 0 {
		c.NoisePatterns = []string{
			`\[.*?\]`,
			`\(.*?\)`,
			`\s(v|vol)\.?\s*\d+`,
			`\s(ch|ch\.|chapter)\s*\d+`,
			`\s(part|pt)\.?\s*\d+`,
		}
	}
	if len(c.StopWords) == 0 {
		c.StopWords = []string{"the", "a", "an", "and", "of"}
	}
	return c
}

func (n *Normalizer) compile() {
	// Build character class. Put - at end so it's literal, not a range.
	var class []string
	for _, s := range n.config.Separators {
		if s == "-" {
			continue // handle separately at end
		}
		class = append(class, regexp.QuoteMeta(s))
	}
	class = append(class, "-") // - at end = literal
	sepPattern := "[" + strings.Join(class, "") + "]"
	n.sepRe = regexp.MustCompile(sepPattern)

	noisePattern := "(?i)" + strings.Join(n.config.NoisePatterns, "|")
	n.noiseRe = regexp.MustCompile(noisePattern)

	n.stopSet = make(map[string]bool, len(n.config.StopWords))
	for _, w := range n.config.StopWords {
		n.stopSet[n.caseFold.String(w)] = true
	}
}

// Configure replaces the rule set and re-compiles internal matchers.
func (n *Normalizer) Configure(config NormalizationConfig) {
	n.config = normalizeConfig(config)
	n.compile()
}

var extRe = regexp.MustCompile(`(?i)\.(cbz|cbr|cb7|zip|rar|7z|tar\.gz|mkv|mp4|avi|m4v|mov|wmv|flv|pdf|epub|mobi|azw3|djvu|jpg|jpeg|png|gif|webp|bmp|tiff?)$`)

// NFKC applies Unicode NFKC normalization to s. NFKC decomposes characters to
// their canonical form and maps compatibility equivalents (fullwidth letters,
// ligatures, typographic symbols, etc.) to their ASCII equivalents.
// Non-Latin scripts (Hangul, CJK, etc.) are preserved — they are already in
// their canonical form.
func NFKC(s string) string {
	return norm.NFKC.String(s)
}

// supplementalFold maps characters that NFKC does not handle but are common in
// manga/media titles: curly quotes, en/em dashes, wave dashes, and
// typographic symbols. Applied after NFKC.
var supplementalFold = map[rune]rune{
	// Curly single quotes → ASCII apostrophe
	'\u2018': '\'', // ' LEFT SINGLE QUOTATION MARK
	'\u2019': '\'', // ' RIGHT SINGLE QUOTATION MARK
	'\u02BC': '\'', // ʼ MODIFIER LETTER APOSTROPHE
	'\u2032': '\'', // ′ PRIME

	// Curly double quotes → ASCII double quote
	'\u201C': '"', // " LEFT DOUBLE QUOTATION MARK
	'\u201D': '"', // " RIGHT DOUBLE QUOTATION MARK
	'\u00AB': '"', // « GUILLEMET LEFT
	'\u00BB': '"', // » GUILLEMET RIGHT
	'\u2033': '"', // ″ DOUBLE PRIME

	// En/em dashes → ASCII hyphen-minus
	'\u2010': '-', // ‐ HYPHEN
	'\u2011': '-', // ‑ NON-BREAKING HYPHEN
	'\u2012': '-', // ‒ FIGURE DASH
	'\u2013': '-', // – EN DASH
	'\u2014': '-', // — EM DASH
	'\u2015': '-', // ― HORIZONTAL BAR
	'\u2043': '-', // ⁃ HYPHEN BULLET
	'\uFE58': '-', // ﹘ SMALL EM DASH
	'\uFE63': '-', // ﹣ SMALL HYPHEN-MINUS

	// Wave dash → ASCII tilde
	'\u301C': '~', // 〜 WAVE DASH
	'\u3030': '~', // 〰 WAVY DASH

	// Typographic symbols → ASCII
	'\u00A9': '(', // © → (
	'\u00AE': ')', // ® → )
	'\u00B0': 'o', // ° → o
	'\u00D7': 'x', // × → x
	'\u00F7': '/', // ÷ → /
}

// FoldUnicode applies NFKC normalization followed by a supplemental fold for
// characters that NFKC does not handle (curly quotes, en/em dashes, wave
// dashes, typographic symbols). Use this at storage boundaries to canonicalise
// text without altering casing, separators, or stop words. Preserves Hangul,
// CJK, and other non-Latin scripts.
func FoldUnicode(s string) string {
	s = NFKC(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if mapped, ok := supplementalFold[r]; ok {
			b.WriteRune(mapped)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Normalize returns the canonical form of name.
func (n *Normalizer) Normalize(name string) (string, error) {
	s := name

	// NFKC normalize + supplemental fold for characters NFKC misses
	s = FoldUnicode(s)

	s = extRe.ReplaceAllString(s, "")
	s = n.sepRe.ReplaceAllString(s, " ")
	s = n.noiseRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)

	// Collapse multiple spaces into one
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}

	words := strings.Fields(s)
	filtered := make([]string, 0, len(words))
	for _, w := range words {
		if !n.stopSet[n.caseFold.String(w)] {
			filtered = append(filtered, w)
		}
	}

	return n.caseFold.String(strings.Join(filtered, " ")), nil
}

// MustNormalize panics if Normalize returns an error.
func (n *Normalizer) MustNormalize(name string) string {
	f, err := n.Normalize(name)
	if err != nil {
		panic(err)
	}
	return f
}

// NormalizeAltTitlesJSON normalizes every title value inside an alt_title JSON
// blob. Works with both formats:
//
//	Structured: {"primary":{"en":"..."},"alts":[{"ja":"..."}]}
//	Flat:       {"en":"...","ja":"..."}
//
// Returns the re-serialised JSON with all values fully normalized (NFKC → custom
// steps → case fold). Returns empty string if the input is empty or unparseable.
// This is media-agnostic — use it for manga, anime, light novels, etc.
// Output is deterministic (sorted map keys).
func NormalizeAltTitlesJSON(raw string) string {
	if raw == "" {
		return ""
	}

	n := New(NormalizationConfig{})

	// Try structured format first: {"primary":{...},"alts":[{...},...]}
	var structured struct {
		Primary map[string]string   `json:"primary"`
		Alts    []map[string]string `json:"alts"`
	}
	if err := json.Unmarshal([]byte(raw), &structured); err == nil && structured.Primary != nil {
		normalized := make(map[string]string)
		for lang, t := range structured.Primary {
			normalized[lang] = n.MustNormalize(t)
		}
		structured.Primary = normalized
		for i, alt := range structured.Alts {
			normalizedAlt := make(map[string]string)
			for lang, t := range alt {
				normalizedAlt[lang] = n.MustNormalize(t)
			}
			structured.Alts[i] = normalizedAlt
		}
		return marshalSorted(structured)
	}

	// Try flat format: {"en":"...","ja":"..."}
	var flat map[string]string
	if err := json.Unmarshal([]byte(raw), &flat); err == nil {
		normalized := make(map[string]string)
		for lang, t := range flat {
			normalized[lang] = n.MustNormalize(t)
		}
		return marshalMapSorted(normalized)
	}

	return ""
}

// NormalizeAllTitlesJSON applies NFKC normalization and the supplemental fold
// to every title value inside an alt_title JSON blob. Preserves case and words
// — use this at storage boundaries where you want compatibility characters
// normalized but titles preserved in readable form. Returns empty string if
// input is empty or unparseable. Media-agnostic. Output is deterministic
// (sorted map keys).
func NormalizeAllTitlesJSON(raw string) string {
	if raw == "" {
		return ""
	}

	// Try structured format first: {"primary":{...},"alts":[{...},...]}
	var structured struct {
		Primary map[string]string   `json:"primary"`
		Alts    []map[string]string `json:"alts"`
	}
	if err := json.Unmarshal([]byte(raw), &structured); err == nil && structured.Primary != nil {
		normalized := make(map[string]string)
		for lang, t := range structured.Primary {
			normalized[lang] = FoldUnicode(t)
		}
		structured.Primary = normalized
		for i, alt := range structured.Alts {
			normalizedAlt := make(map[string]string)
			for lang, t := range alt {
				normalizedAlt[lang] = FoldUnicode(t)
			}
			structured.Alts[i] = normalizedAlt
		}
		return marshalSorted(structured)
	}

	// Try flat format: {"en":"...","ja":"..."}
	var flat map[string]string
	if err := json.Unmarshal([]byte(raw), &flat); err == nil {
		normalized := make(map[string]string)
		for lang, t := range flat {
			normalized[lang] = FoldUnicode(t)
		}
		return marshalMapSorted(normalized)
	}

	return ""
}

// marshalSorted marshals a struct to JSON with sorted map keys for deterministic output.
func marshalSorted(v interface{}) string {
	// Marshal the struct normally, then re-sort the top-level maps
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}

	// For the structured format, we need to re-marshal with sorted keys
	var structured struct {
		Primary map[string]string   `json:"primary"`
		Alts    []map[string]string `json:"alts"`
	}
	if err := json.Unmarshal(data, &structured); err != nil {
		return string(data)
	}

	// Build JSON manually with sorted keys
	var buf strings.Builder
	buf.WriteString(`{"primary":`)
	buf.WriteString(marshalMapSorted(structured.Primary))
	buf.WriteString(`,"alts":[`)
	for i, alt := range structured.Alts {
		if i > 0 {
			buf.WriteString(`,`)
		}
		buf.WriteString(marshalMapSorted(alt))
	}
	buf.WriteString(`]}`)
	return buf.String()
}

// marshalMapSorted marshals a string map to JSON with sorted keys.
func marshalMapSorted(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteString(`{`)
	for i, k := range keys {
		if i > 0 {
			buf.WriteString(`,`)
		}
		keyJSON, _ := json.Marshal(k)
		valJSON, _ := json.Marshal(m[k])
		buf.Write(keyJSON)
		buf.WriteString(`:`)
		buf.Write(valJSON)
	}
	buf.WriteString(`}`)
	return buf.String()
}
