package pipeline

import (
	"testing"

	"github.com/adamfitz/scrape/internal/mangadex"
	"github.com/adamfitz/scrape/internal/normalize"
)

func TestQueryMatchesAnyTitle_DottedQuery(t *testing.T) {
	n := normalize.New(normalize.NormalizationConfig{})
	query := "am i actually strongest"

	result := mangadex.MangaResult{
		ID: "9484b1fd-0271-4c9b-b096-7e313823058e",
		Attributes: mangadex.MangaAttributes{
			Title: map[string]string{
				"ja-ro": "Jitsu wa Ore, Saikyou deshita?",
			},
			AltTitles: []map[string]string{
				{"en": "Am I Actually the Strongest?"},
			},
		},
	}

	if !queryMatchesAnyTitle(query, result, n) {
		t.Errorf("expected match for query %q against alt title", query)
	}
}
