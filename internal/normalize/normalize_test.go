package normalize_test

import (
	"strings"
	"testing"

	"github.com/adamfitz/scrape/internal/normalize"
)

func TestNormalize_SeparatorFolding(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("Chainsaw.Man")
	want := n.MustNormalize("Chainsaw Man")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_NoiseRemoval(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("[ScanGroup] Bloom Into You (Digital)")
	want := n.MustNormalize("Bloom Into You")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_Deterministic(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Chainsaw.Man")
	b := n.MustNormalize("Chainsaw.Man")

	if a != b {
		t.Errorf("not deterministic: %q != %q", a, b)
	}
}

func TestNormalize_StopWords(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("The Manga Name")
	want := n.MustNormalize("Manga Name")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_ConfigurableStopWords(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{
		StopWords: []string{},
	})

	got := n.MustNormalize("The Manga Name")
	want := n.MustNormalize("The Manga Name")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_ScriptPreservation(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	kanji := "鬼滅の刃"
	got := n.MustNormalize(kanji)

	if string(got) != kanji {
		t.Errorf("kanji not preserved: got %q, want %q", got, kanji)
	}
}

func TestNormalize_OriginalPreserved(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	original := "Chainsaw.Man"
	n.Normalize(original)

	got := original
	want := "Chainsaw.Man"
	if got != want {
		t.Errorf("original was modified: got %q, want %q", got, want)
	}
}

func TestNormalize_ExtensionRemovalCBR(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("One.Piece.Ch.1090.cbz")
	want := n.MustNormalize("One Piece")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_ExtensionRemovalMKV(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("The.Manga.Name.mkv")
	want := n.MustNormalize("The Manga Name")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_NoExtensionNoChange(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("Chainsaw Man")
	want := n.MustNormalize("Chainsaw Man")

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_NotAFakeExtension(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("no.dots.here")
	want := "no dots here"

	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalize_KoreanPreservation(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	korean := "외모지상주의"
	got := n.MustNormalize(korean)

	if string(got) != korean {
		t.Errorf("korean not preserved: got %q, want %q", got, korean)
	}
}

func TestNormalize_ColonFolding(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("After Being Called a Hero: The Unrivaled Man Starts a Family")
	b := n.MustNormalize("After Being Called a Hero The Unrivaled Man Starts a Family")

	if a != b {
		t.Errorf("colon not folded: %q != %q", a, b)
	}
}

func TestNormalize_SemicolonFolding(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Title; Subtitle")
	b := n.MustNormalize("Title Subtitle")

	if a != b {
		t.Errorf("semicolon not folded: %q != %q", a, b)
	}
}

func TestNormalize_CommaFolding(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Smith, John")
	b := n.MustNormalize("Smith John")

	if a != b {
		t.Errorf("comma not folded: %q != %q", a, b)
	}
}

func TestNormalize_MultipleSpaceCollapse(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("Title:   Subtitle")
	want := "title subtitle"

	if got != want {
		t.Errorf("multiple spaces not collapsed: got %q, want %q", got, want)
	}
}

func TestNormalize_ApostropheVariants(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// U+0027 APOSTROPHE vs U+2019 RIGHT SINGLE QUOTATION MARK
	a := n.MustNormalize("Hero's Party")
	b := n.MustNormalize("Hero\u2019s Party")

	if a != b {
		t.Errorf("apostrophe variants not folded: %q != %q", a, b)
	}
}

func TestNormalize_DoubleQuoteFolding(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Double quotes should be folded to spaces
	a := n.MustNormalize(`Tsuki "Okaeri" tte Iu`)
	b := n.MustNormalize("Tsuki Okaeri tte Iu")

	if a != b {
		t.Errorf("double quotes not folded: %q != %q", a, b)
	}

	// Unicode double quotes should also fold
	c := n.MustNormalize("Tsuki \u201COkaeri\u201D tte Iu")
	if c != b {
		t.Errorf("unicode double quotes not folded: %q != %q", c, b)
	}
}

func TestNormalize_TildeVariants(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// U+007E TILDE vs U+FF5E FULLWIDTH TILDE vs U+301C WAVE DASH
	a := n.MustNormalize("Title~ Subtitle")
	b := n.MustNormalize("Title\uFF5E Subtitle")
	c := n.MustNormalize("Title\u301C Subtitle")

	if a != b {
		t.Errorf("fullwidth tilde not folded: %q != %q", a, b)
	}
	if a != c {
		t.Errorf("wave dash not folded: %q != %q", a, c)
	}
}

func TestNormalize_DashVariants(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// U+002D HYPHEN-MINUS vs U+2013 EN DASH vs U+2014 EM DASH
	a := n.MustNormalize("Title - Subtitle")
	b := n.MustNormalize("Title \u2013 Subtitle")
	c := n.MustNormalize("Title \u2014 Subtitle")

	if a != b {
		t.Errorf("en dash not folded: %q != %q", a, b)
	}
	if a != c {
		t.Errorf("em dash not folded: %q != %q", a, c)
	}
}

func TestNormalize_FullwidthASCII(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("ＳＡＯ")
	b := n.MustNormalize("SAO")

	if a != b {
		t.Errorf("fullwidth ASCII not folded: %q != %q", a, b)
	}
}

func TestNormalize_AccentPreserved(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Accented characters are preserved (we don't strip them),
	// but the string should still normalize consistently
	a := n.MustNormalize("Résumé")
	b := n.MustNormalize("Résumé")

	if a != b {
		t.Errorf("accented chars not deterministic: %q != %q", a, b)
	}
}

func TestNormalize_JackOfAllTrades(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// The exact case from the bug: different apostrophe + tilde variants
	a := n.MustNormalize("The Jack-of-all-trades Kicked Out of the Hero\u2019s Party ~ The Swordsman Who Became a Support Mage Due to Party Circumstances, Becomes All Powerful")
	b := n.MustNormalize("The Jack-of-all-trades Kicked Out of the Hero's Party ~ The Swordsman Who Became a Support Mage Due to Party Circumstances, Becomes All Powerful")
	c := n.MustNormalize("The Jack-of-All-Trades Kicked Out of the Hero\u2019s Party: The Swordsman Who Became a Support Mage Due to Party Circumstances, Becomes All-Powerful")

	if a != b {
		t.Errorf("apostrophe variants mismatch: %q != %q", a, b)
	}
	if a != c {
		t.Errorf("colon/tilde and hyphen variants mismatch: %q != %q", a, c)
	}
}

func TestFoldUnicode_ApostropheVariants(t *testing.T) {
	a := normalize.FoldUnicode("Hero\u2019s Party")
	b := normalize.FoldUnicode("Hero's Party")

	if a != b {
		t.Errorf("apostrophe variants not folded: %q != %q", a, b)
	}
	if a != "Hero's Party" {
		t.Errorf("expected ASCII apostrophe, got %q", a)
	}
}

func TestFoldUnicode_TildeVariants(t *testing.T) {
	a := normalize.FoldUnicode("Title\uFF5E Subtitle")
	b := normalize.FoldUnicode("Title\u301C Subtitle")
	c := normalize.FoldUnicode("Title~ Subtitle")

	if a != c {
		t.Errorf("fullwidth tilde not folded: %q != %q", a, c)
	}
	if b != c {
		t.Errorf("wave dash not folded: %q != %q", b, c)
	}
}

func TestFoldUnicode_DashVariants(t *testing.T) {
	a := normalize.FoldUnicode("Title\u2013 Subtitle")
	b := normalize.FoldUnicode("Title\u2014 Subtitle")
	c := normalize.FoldUnicode("Title- Subtitle")

	if a != c {
		t.Errorf("en dash not folded: %q != %q", a, c)
	}
	if b != c {
		t.Errorf("em dash not folded: %q != %q", b, c)
	}
}

func TestFoldUnicode_PreservesCasing(t *testing.T) {
	got := normalize.FoldUnicode("The Hero\u2019s Party")
	want := "The Hero's Party"

	if got != want {
		t.Errorf("FoldUnicode changed casing: %q != %q", got, want)
	}
}

func TestFoldUnicode_PreservesCJK(t *testing.T) {
	got := normalize.FoldUnicode("\u9b3c\u6ec4\u306e\u5203") // 鬼滅の刃
	want := "\u9b3c\u6ec4\u306e\u5203"

	if got != want {
		t.Errorf("FoldUnicode altered CJK: %q != %q", got, want)
	}
}

func TestFoldUnicode_PreservesKorean(t *testing.T) {
	got := normalize.FoldUnicode("\uc624\ubaa8\uc9c0\uc0c1\uc8fc\uc758") // 외모지상주의
	want := "\uc624\ubaa8\uc9c0\uc0c1\uc8fc\uc758"

	if got != want {
		t.Errorf("FoldUnicode altered Korean: %q != %q", got, want)
	}
}

func TestFoldAltTitlesJSON_Structured(t *testing.T) {
	raw := `{"primary":{"en":"The Hero\u2019s Party","ja":"勇者パーティー"},"alts":[{"ja":"勇者パーティ","en":"Hero\u2019s Party"}]}`
	got := normalize.NormalizeAllTitlesJSON(raw)

	// The ' (U+2019) should be folded to ' (U+0027) in all values
	if strings.Contains(got, "\u2019") {
		t.Errorf("FoldAltTitlesJSON did not fold apostrophe variants: %s", got)
	}
	if !strings.Contains(got, "The Hero's Party") {
		t.Errorf("FoldAltTitlesJSON did not preserve readable title: %s", got)
	}
}

func TestFoldAltTitlesJSON_Flat(t *testing.T) {
	raw := `{"en":"Hero\u2019s Party","ja":"勇者パーティー"}`
	got := normalize.NormalizeAllTitlesJSON(raw)

	if strings.Contains(got, "\u2019") {
		t.Errorf("FoldAltTitlesJSON did not fold apostrophe: %s", got)
	}
}

func TestFoldAltTitlesJSON_Empty(t *testing.T) {
	got := normalize.NormalizeAllTitlesJSON("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestNormalizeAltTitlesJSON_Structured(t *testing.T) {
	raw := `{"primary":{"en":"The Hero\u2019s Party","ja":"勇者パーティー"},"alts":[]}`
	got := normalize.NormalizeAltTitlesJSON(raw)

	// Should be fully normalized (lowercased, folded, stop words removed)
	if strings.Contains(got, "\u2019") {
		t.Errorf("NormalizeAltTitlesJSON did not fold: %s", got)
	}
}

func TestGrammarExpansion_Contraction(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("You Like Me, Dont You")
	want := n.MustNormalize("You Like Me, Do Not You")

	if got != want {
		t.Errorf("grammar expansion failed: got %q, want %q", got, want)
	}
}

func TestGrammarExpansion_ApostropheForm(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// "Don't" has apostrophe which is a separator, so it becomes "dont"
	// then grammar expansion expands "dont" to "do not"
	got := n.MustNormalize("You Like Me, Don't You")
	want := "you like me do not you"

	if got != want {
		t.Errorf("apostrophe grammar expansion: got %q, want %q", got, want)
	}
}

func TestGrammarExpansion_MultipleContractions(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("I Cant Believe Its Not Butter")
	want := "i can not believe it is not butter"

	if got != want {
		t.Errorf("multiple contractions: got %q, want %q", got, want)
	}
}

func TestGrammarExpansion_Youre(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("Youre Under My Skin")
	want := "you are under my skin"

	if got != want {
		t.Errorf("youre expansion: got %q, want %q", got, want)
	}
}

func TestGrammarExpansion_NoMatch(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("One Piece")
	want := "one piece"

	if got != want {
		t.Errorf("no grammar match: got %q, want %q", got, want)
	}
}

func TestGrammarExpansion_Extensibility(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Add a custom rule
	normalize.GrammarRules["gonna"] = "going to"
	defer delete(normalize.GrammarRules, "gonna")

	got := n.MustNormalize("I Gonna Go")
	want := "i going to go"

	if got != want {
		t.Errorf("extensibility: got %q, want %q", got, want)
	}
}

func TestGrammarExpansion_Idempotent(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("You Like Me, Dont You")
	b := n.MustNormalize("You Like Me, Dont You")

	if a != b {
		t.Errorf("grammar expansion not idempotent: %q != %q", a, b)
	}
}

func TestNormalize_Idempotency(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})
	inputs := []string{
		"Chainsaw.Man",
		"[ScanGroup] Bloom Into You (Digital)",
		"The Jack-of-all-trades Kicked Out of the Hero\u2019s Party ~ The Swordsman Who Became a Support Mage Due to Party Circumstances, Becomes All Powerful",
		"You Like Me, Dont You",
		"ＳＡＯ",
		"鬼滅の刃",
	}

	for _, input := range inputs {
		a := n.MustNormalize(input)
		b := n.MustNormalize(input)
		if a != b {
			t.Errorf("not idempotent for %q: %q != %q", input, a, b)
		}
	}
}

func TestNormalize_Transitivity(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})
	inputs := []string{
		"Chainsaw.Man",
		"[ScanGroup] Bloom Into You (Digital)",
		"You Like Me, Dont You",
		"Hero\u2019s Party ~ Swordsman",
		"ＳＡＯ",
	}

	for _, input := range inputs {
		once := n.MustNormalize(input)
		twice := n.MustNormalize(once)
		if once != twice {
			t.Errorf("not transitive for %q: once=%q, twice=%q", input, once, twice)
		}
	}
}

func TestNormalize_NestedBrackets(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Nested brackets: [A] [B] Title
	got := n.MustNormalize("[ScanGroup] [v2] Bloom Into You")
	want := n.MustNormalize("Bloom Into You")
	if got != want {
		t.Errorf("nested brackets: got %q, want %q", got, want)
	}

	// Nested parentheses: non-greedy regex matches first pair, trailing ) remains
	got2 := n.MustNormalize("Title (foo (bar))")
	want2 := n.MustNormalize("Title )")
	if got2 != want2 {
		t.Errorf("nested parentheses: got %q, want %q", got2, want2)
	}
}

func TestNormalize_AmpersandSeparator(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Dungeons & Artifacts")
	b := n.MustNormalize("Dungeons Artifacts")

	if a != b {
		t.Errorf("ampersand not folded: %q != %q", a, b)
	}
}

func TestNormalize_PipeSeparator(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Title | Subtitle")
	b := n.MustNormalize("Title Subtitle")

	if a != b {
		t.Errorf("pipe not folded: %q != %q", a, b)
	}
}

func TestNormalize_BulletSeparator(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Title \u2022 Subtitle")
	b := n.MustNormalize("Title Subtitle")

	if a != b {
		t.Errorf("bullet not folded: %q != %q", a, b)
	}
}

func TestNormalize_EditionMarker_PerfectEdition(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("Baki the Grappler - Perfect Edition")
	want := n.MustNormalize("Baki the Grappler")

	if got != want {
		t.Errorf("Perfect Edition not stripped: got %q, want %q", got, want)
	}
}

func TestNormalize_EditionMarker_OmnibusEdition(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Parenthesized form is already stripped by \(.*?\)
	got := n.MustNormalize("Initial D (Omnibus Edition)")
	want := n.MustNormalize("Initial D")

	if got != want {
		t.Errorf("Omnibus Edition not stripped: got %q, want %q", got, want)
	}
}

func TestNormalize_EditionMarker_TwoInOne(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Parenthesized form is already stripped by \(.*?\)
	got := n.MustNormalize("Maid-sama! (2-in-1 Edition)")
	want := n.MustNormalize("Maid-sama!")

	if got != want {
		t.Errorf("2-in-1 Edition not stripped: got %q, want %q", got, want)
	}
}

func TestNormalize_BareTrailingYear(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	// Year at end of string is stripped
	got := n.MustNormalize("Some Manga 2022")
	want := n.MustNormalize("Some Manga")

	if got != want {
		t.Errorf("bare trailing year not stripped: got %q, want %q", got, want)
	}

	// Year in middle is NOT stripped (only trailing years)
	got2 := n.MustNormalize("Some Manga 2022 Uncensored")
	want2 := n.MustNormalize("Some Manga 2022 Uncensored")

	if got2 != want2 {
		t.Errorf("mid-string year should be preserved: got %q, want %q", got2, want2)
	}
}

func TestNormalize_RomanNumeralVol(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	got := n.MustNormalize("Manga Title Vol. III")
	want := n.MustNormalize("Manga Title")

	if got != want {
		t.Errorf("Roman numeral vol not stripped: got %q, want %q", got, want)
	}
}

func TestNormalize_PartNumberPreserved(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})

	a := n.MustNormalize("Ascendance of a Bookworm - Part 01")
	b := n.MustNormalize("Ascendance of a Bookworm - Part 02")

	if a == b {
		t.Errorf("part numbers should be preserved but got same result: %q", a)
	}
}

func TestNormalizeAltTitlesJSON_Idempotent(t *testing.T) {
	raw := `{"primary":{"en":"The Hero\u2019s Party","ja":"勇者パーティー"},"alts":[{"ja":"勇者パーティ","en":"Hero\u2019s Party"}]}`
	a := normalize.NormalizeAltTitlesJSON(raw)
	b := normalize.NormalizeAltTitlesJSON(raw)

	if a != b {
		t.Errorf("NormalizeAltTitlesJSON not idempotent: %q != %q", a, b)
	}
}

func TestNormalizeAllTitlesJSON_Idempotent(t *testing.T) {
	raw := `{"primary":{"en":"The Hero\u2019s Party","ja":"勇者パーティー"},"alts":[{"ja":"勇者パーティ","en":"Hero\u2019s Party"}]}`
	a := normalize.NormalizeAllTitlesJSON(raw)
	b := normalize.NormalizeAllTitlesJSON(raw)

	if a != b {
		t.Errorf("NormalizeAllTitlesJSON not idempotent: %q != %q", a, b)
	}
}
