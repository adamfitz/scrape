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
