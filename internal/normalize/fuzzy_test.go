package normalize_test

import (
	"testing"

	"github.com/adamfitz/scrape/internal/normalize"
)

func TestSimilarity_ExactMatch(t *testing.T) {
	got := normalize.Similarity("One Piece", "One Piece")
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestSimilarity_EmptyInputs(t *testing.T) {
	if got := normalize.Similarity("", "test"); got != 0.0 {
		t.Errorf("empty a: expected 0.0, got %f", got)
	}
	if got := normalize.Similarity("test", ""); got != 0.0 {
		t.Errorf("empty b: expected 0.0, got %f", got)
	}
	if got := normalize.Similarity("", ""); got != 0.0 {
		t.Errorf("both empty: expected 0.0, got %f", got)
	}
}

func TestSimilarity_Substring(t *testing.T) {
	got := normalize.Similarity("One Piece", "One")
	if got < 0.9 {
		t.Errorf("substring: expected >= 0.9, got %f", got)
	}
}

func TestSimilarity_CaseInsensitive(t *testing.T) {
	got := normalize.Similarity("ONE PIECE", "one piece")
	if got != 1.0 {
		t.Errorf("case insensitive: expected 1.0, got %f", got)
	}
}

func TestSimilarity_UnicodeVariants(t *testing.T) {
	got := normalize.Similarity("Hero\u2019s Party", "Hero's Party")
	if got != 1.0 {
		t.Errorf("unicode variants: expected 1.0, got %f", got)
	}
}

func TestSimilarity_CompletelyDifferent(t *testing.T) {
	got := normalize.Similarity("One Piece", "Naruto")
	if got >= 0.5 {
		t.Errorf("completely different: expected < 0.5, got %f", got)
	}
}

func TestSimilarity_WordReordering(t *testing.T) {
	got := normalize.Similarity("You Dont You", "Dont You You")
	if got < 0.8 {
		t.Errorf("word reordering: expected >= 0.8, got %f", got)
	}
}

func TestSimilarity_Typo(t *testing.T) {
	got := normalize.Similarity("Yuusha Party", "Yusha Party")
	if got < 0.85 {
		t.Errorf("typo: expected >= 0.85, got %f", got)
	}
}

func TestSimilarity_GrammarVariation(t *testing.T) {
	got := normalize.Similarity("You Like Me, Dont You", "You Like Me, Don't You")
	if got < 0.85 {
		t.Errorf("grammar variation: expected >= 0.85, got %f", got)
	}
}

func TestMatch_AboveThreshold(t *testing.T) {
	if !normalize.Match("One Piece", "One Piece", 0.85) {
		t.Error("expected match above threshold")
	}
}

func TestMatch_BelowThreshold(t *testing.T) {
	if normalize.Match("One Piece", "Naruto", 0.85) {
		t.Error("expected no match below threshold")
	}
}

func TestBestMatch_FindsCorrectCandidate(t *testing.T) {
	candidates := []string{"One Piece", "One Punch Man", "One Piece Film"}
	idx, score, ok := normalize.BestMatch("One Piece", candidates, 0.85)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if idx != 0 {
		t.Errorf("expected index 0, got %d", idx)
	}
	if score < 0.9 {
		t.Errorf("expected score >= 0.9, got %f", score)
	}
}

func TestBestMatch_NoMatchAboveThreshold(t *testing.T) {
	candidates := []string{"Naruto", "Bleach", "Dragon Ball"}
	_, _, ok := normalize.BestMatch("One Piece", candidates, 0.85)
	if ok {
		t.Error("expected ok=false for no match above threshold")
	}
}

func TestBestMatch_EmptyCandidates(t *testing.T) {
	_, _, ok := normalize.BestMatch("One Piece", []string{}, 0.85)
	if ok {
		t.Error("expected ok=false for empty candidates")
	}
}

func TestBestMatch_SingleCandidate(t *testing.T) {
	candidates := []string{"You're Under My Skin!"}
	idx, score, ok := normalize.BestMatch("Youre under my Skin", candidates, 0.85)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if idx != 0 {
		t.Errorf("expected index 0, got %d", idx)
	}
	if score < 0.85 {
		t.Errorf("expected score >= 0.85, got %f", score)
	}
}

func TestBestMatch_MultipleMatches_PicksHighest(t *testing.T) {
	candidates := []string{
		"You Like Me, Don't You?",
		"You Like Me, Don't You (Digital)",
		"Something Else",
	}
	idx, _, ok := normalize.BestMatch("You Like Me, Dont You", candidates, 0.85)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if idx != 0 && idx != 1 {
		t.Errorf("expected index 0 or 1, got %d", idx)
	}
}

func TestBestMatch_ExactMatchInList(t *testing.T) {
	candidates := []string{"Naruto", "One Piece", "Bleach"}
	idx, score, ok := normalize.BestMatch("One Piece", candidates, 0.85)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %f", score)
	}
}

func TestSimilarity_ShortStrings(t *testing.T) {
	got := normalize.Similarity("D", "D!")
	if got < 0.8 {
		t.Errorf("short strings: expected >= 0.8, got %f", got)
	}
}
