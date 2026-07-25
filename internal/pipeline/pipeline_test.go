package pipeline_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/adamfitz/scrape/internal/database"
	"github.com/adamfitz/scrape/internal/pipeline"
)

func tempDB(t *testing.T) *database.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func tempPipeline(t *testing.T) *pipeline.Pipeline {
	t.Helper()
	db := tempDB(t)
	return pipeline.New(db, pipeline.Options{})
}

func TestLookup_ExactMatch(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "a1",
		AltTitle: `{"en":"One Piece","ja":"ワンピース"}`,
	})

	got, err := p.Lookup(database.MediaTypeManga, "One Piece")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got == nil {
		t.Fatal("expected match, got nil")
	}
	if got.Title != "One Piece" {
		t.Errorf("title = %q, want %q", got.Title, "One Piece")
	}
}

func TestCaseInsensitiveLookup(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "c1",
	})

	got, err := p.Lookup(database.MediaTypeManga, "one piece")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got == nil {
		t.Fatal("expected match, got nil")
	}
}

func TestLookup_AltTitleMatch(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "a2",
		AltTitle: `{"ja":"ワンピース"}`,
	})

	got, err := p.Lookup(database.MediaTypeManga, "ワンピース")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got == nil {
		t.Fatal("expected match via alt title, got nil")
	}
}

func TestLookup_NotFound(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "n1",
	})

	got, err := p.Lookup(database.MediaTypeManga, "Naruto")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBatchLookup(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "b1"})
	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "b2"})

	titles := []string{"One Piece", "Naruto", "Bleach", "Death Note"}
	found, missing := p.BatchLookup(database.MediaTypeManga, titles)

	if len(found) != 2 {
		t.Errorf("found %d, want 2", len(found))
	}
	if len(missing) != 2 {
		t.Errorf("missing %d, want 2", len(missing))
	}
	if _, ok := found["One Piece"]; !ok {
		t.Error("expected 'One Piece' in found")
	}
	if _, ok := found["Naruto"]; !ok {
		t.Error("expected 'Naruto' in found")
	}
}

func TestIngest_Insert(t *testing.T) {
	p := tempPipeline(t)

	m := &database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "ing1",
		AltTitle: `{"en":"One Piece","ja":"ワンピース"}`,
	}

	id, err := p.Ingest(m)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	got, _ := p.Lookup(database.MediaTypeManga, "One Piece")
	if got == nil {
		t.Fatal("expected record after ingest, got nil")
	}
}

func TestIngest_Update(t *testing.T) {
	p := tempPipeline(t)

	m := &database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Old Title",
		SourceID: "upd1",
	}
	p.Ingest(m)

	m.Title = "New Title"
	p.Ingest(m)

	got, _ := p.DB().GetMediaBySourceID("upd1")
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.Title != "New Title" {
		t.Errorf("title = %q, want %q", got.Title, "New Title")
	}
}

func TestIngest_NormalizesTitle(t *testing.T) {
	p := tempPipeline(t)

	m := &database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Hero\u2019s Party",
		SourceID: "norm1",
	}
	p.Ingest(m)

	got, _ := p.DB().GetMediaBySourceID("norm1")
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.Title != "Hero's Party" {
		t.Errorf("title not normalised: %q, want %q", got.Title, "Hero's Party")
	}
}

func TestIngest_NormalizesAltTitle(t *testing.T) {
	p := tempPipeline(t)

	m := &database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Test",
		SourceID: "norm2",
		AltTitle: `{"primary":{"en":"Hero\u2019s Party"},"alts":[]}`,
	}
	p.Ingest(m)

	got, _ := p.DB().GetMediaBySourceID("norm2")
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.AltTitle != `{"primary":{"en":"Hero's Party"},"alts":[]}` {
		t.Errorf("alt_title not normalised: %s", got.AltTitle)
	}
}

func TestNormalizeAllTitles(t *testing.T) {
	p := tempPipeline(t)

	p.DB().InsertMedia(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Hero\u2019s Party \u301C Subtitle",
		SourceID: "na1",
	})

	updated, err := p.NormalizeAllTitles()
	if err != nil {
		t.Fatalf("NormalizeAllTitles: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated %d, want 1", updated)
	}

	got, _ := p.DB().GetMediaBySourceID("na1")
	if got.Title != "Hero's Party ~ Subtitle" {
		t.Errorf("title = %q, want %q", got.Title, "Hero's Party ~ Subtitle")
	}
}

func TestNormalizeAllTitles_Idempotent(t *testing.T) {
	p := tempPipeline(t)

	p.DB().InsertMedia(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Hero\u2019s Party",
		SourceID: "idp1",
	})

	p.NormalizeAllTitles()

	updated, err := p.NormalizeAllTitles()
	if err != nil {
		t.Fatalf("NormalizeAllTitles: %v", err)
	}
	if updated != 0 {
		t.Errorf("second run updated %d, want 0 (should be idempotent)", updated)
	}
}

func TestQueryForAPI(t *testing.T) {
	q := pipeline.QueryForAPI("One Piece [Official]")
	if q == "" {
		t.Error("expected non-empty query")
	}
}

func TestDeduplicate(t *testing.T) {
	titles := []string{"One Piece", "one piece", "ONE PIECE", "Naruto"}
	unique := pipeline.Deduplicate(titles)
	if len(unique) != 2 {
		t.Errorf("deduplicated %d titles, want 2", len(unique))
	}
}

func TestTitleOrFallback(t *testing.T) {
	altTitle := `{"primary":{"en":"One Piece","ja":"ワンピース"},"alts":[{"zh":"海贼王"}]}`

	got := pipeline.TitleOrFallback(altTitle, "en", "fallback")
	if got != "One Piece" {
		t.Errorf("got %q, want %q", got, "One Piece")
	}

	got = pipeline.TitleOrFallback(altTitle, "ja", "fallback")
	if got != "ワンピース" {
		t.Errorf("got %q, want %q", got, "ワンピース")
	}

	got = pipeline.TitleOrFallback(altTitle, "fr", "fallback")
	if got != "One Piece" {
		t.Errorf("got %q, want %q (should fallback to first available)", got, "One Piece")
	}

	got = pipeline.TitleOrFallback("", "en", "fallback")
	if got != "fallback" {
		t.Errorf("got %q, want %q", got, "fallback")
	}
}

func TestBatchLookup_UnicodeVariants(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Hero\u2019s Party",
		SourceID: "uv1",
	})

	found, missing := p.BatchLookup(database.MediaTypeManga, []string{"Hero's Party"})
	if len(found) != 1 {
		t.Errorf("found %d, want 1 (Unicode variants should match after normalisation)", len(found))
	}
	if len(missing) != 0 {
		t.Errorf("missing %d, want 0", len(missing))
	}
}

// --- New tests for Pipeline struct and error types ---

func TestLookupAPI_LocalHit(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "api1",
	})

	result, err := p.LookupAPI(database.MediaTypeManga, "One Piece", pipeline.LookupOptions{LocalOnly: true})
	if err != nil {
		t.Fatalf("LookupAPI: %v", err)
	}
	if result.Source != pipeline.SourceDB {
		t.Errorf("source = %q, want %q", result.Source, pipeline.SourceDB)
	}
	if result.Media == nil {
		t.Fatal("expected media, got nil")
	}
	if result.Media.Title != "One Piece" {
		t.Errorf("title = %q, want %q", result.Media.Title, "One Piece")
	}
}

func TestLookupAPI_LocalOnly_NotFound(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "api2",
	})

	result, err := p.LookupAPI(database.MediaTypeManga, "Naruto", pipeline.LookupOptions{LocalOnly: true})
	if err != nil {
		t.Fatalf("LookupAPI: %v", err)
	}
	if result.Media != nil {
		t.Errorf("expected nil media, got %v", result.Media)
	}
	if result.Error != "not found locally" {
		t.Errorf("error = %q, want %q", result.Error, "not found locally")
	}
}

func TestLookupAPI_CaseInsensitive(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "api3",
	})

	result, err := p.LookupAPI(database.MediaTypeManga, "one piece", pipeline.LookupOptions{LocalOnly: true})
	if err != nil {
		t.Fatalf("LookupAPI: %v", err)
	}
	if result.Media == nil {
		t.Fatal("expected media, got nil")
	}
}

func TestBatchLookupStream_LocalOnly(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "bs1"})
	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "bs2"})

	titles := []string{"One Piece", "Naruto", "Bleach"}
	ch := p.BatchLookupStream(database.MediaTypeManga, titles, pipeline.BatchLookupOptions{LocalOnly: true})

	var results []pipeline.LookupResult
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Source != pipeline.SourceDB {
			t.Errorf("source = %q, want %q", r.Source, pipeline.SourceDB)
		}
		if r.Media == nil {
			t.Errorf("nil media for %s", r.Query)
		}
	}
}

func TestBatchLookupStream_MixedLocal(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "mx1"})

	titles := []string{"One Piece", "Naruto"}
	ch := p.BatchLookupStream(database.MediaTypeManga, titles, pipeline.BatchLookupOptions{LocalOnly: true})

	var results []pipeline.LookupResult
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
	if results[0].Media == nil || results[0].Media.Title != "One Piece" {
		t.Errorf("expected One Piece, got %v", results[0])
	}
}

func TestParseAltTitles(t *testing.T) {
	structured := `{"primary":{"en":"One Piece","ja":"ワンピース"},"alts":[{"zh":"海贼王"}]}`
	data := pipeline.ParseAltTitles(structured)
	if data.Primary["en"] != "One Piece" {
		t.Errorf("primary en = %q, want %q", data.Primary["en"], "One Piece")
	}
	if len(data.Alts) != 1 {
		t.Errorf("alts count = %d, want 1", len(data.Alts))
	}

	flat := `{"en":"One Piece","ja":"ワンピース"}`
	data = pipeline.ParseAltTitles(flat)
	if data.Primary["en"] != "One Piece" {
		t.Errorf("flat primary en = %q, want %q", data.Primary["en"], "One Piece")
	}

	data = pipeline.ParseAltTitles("")
	if data.Primary != nil {
		t.Errorf("empty should return zero value, got %v", data)
	}
}

func TestPipelineError_Kind(t *testing.T) {
	err := &pipeline.PipelineError{Kind: pipeline.ErrNotFound, Message: "title not in DB"}

	var pe *pipeline.PipelineError
	if !errors.As(err, &pe) {
		t.Fatal("errors.As should extract PipelineError")
	}
	if pe.Kind != pipeline.ErrNotFound {
		t.Errorf("kind = %d, want %d", pe.Kind, pipeline.ErrNotFound)
	}
	if pe.Message != "title not in DB" {
		t.Errorf("message = %q, want %q", pe.Message, "title not in DB")
	}
}

func TestPipelineError_Wrap(t *testing.T) {
	inner := errors.New("disk full")
	err := &pipeline.PipelineError{Kind: pipeline.ErrDatabase, Message: "write failed", Err: inner}

	if !errors.Is(err, inner) {
		t.Fatal("errors.Is should find inner error")
	}

	var pe *pipeline.PipelineError
	if !errors.As(err, &pe) {
		t.Fatal("errors.As should extract PipelineError")
	}
	if pe.Kind != pipeline.ErrDatabase {
		t.Errorf("kind = %d, want %d", pe.Kind, pipeline.ErrDatabase)
	}
}

func TestPipelineError_ErrorString(t *testing.T) {
	err := &pipeline.PipelineError{Kind: pipeline.ErrNotFound}
	if got := err.Error(); got != "not found" {
		t.Errorf("Error() = %q, want %q", got, "not found")
	}

	err = &pipeline.PipelineError{Kind: pipeline.ErrAPIFailed, Message: "search failed"}
	if got := err.Error(); got != "API error: search failed" {
		t.Errorf("Error() = %q, want %q", got, "API error: search failed")
	}
}

// --- Title Index Tests ---

func TestIngest_PopulatesIndex(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Naruto",
		SourceID: "idx1",
	})

	size, _ := p.DB().TitleIndexSize(database.MediaTypeManga)
	if size < 1 {
		t.Errorf("index size = %d, want >= 1 after ingest", size)
	}

	// Should be findable via index
	got, _ := p.Lookup(database.MediaTypeManga, "Naruto")
	if got == nil {
		t.Fatal("expected lookup to find record via index")
	}
}

func TestIngest_IndexesAltTitles(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		SourceID: "idx2",
		AltTitle: `{"ja":"ワンピース","en":"One Piece"}`,
	})

	size, _ := p.DB().TitleIndexSize(database.MediaTypeManga)
	if size < 2 {
		t.Errorf("index size = %d, want >= 2 (title + alt)", size)
	}

	// Alt title should be findable
	got, _ := p.Lookup(database.MediaTypeManga, "ワンピース")
	if got == nil {
		t.Fatal("expected lookup to find record via alt title index")
	}
}

func TestIngest_UpdateReindexes(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Old Title",
		SourceID: "idx3",
	})

	// Old title should be findable
	got, _ := p.Lookup(database.MediaTypeManga, "Old Title")
	if got == nil {
		t.Fatal("expected old title to be findable")
	}

	// Update with new title
	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "New Title",
		SourceID: "idx3",
	})

	// New title should be findable
	got, _ = p.Lookup(database.MediaTypeManga, "New Title")
	if got == nil {
		t.Fatal("expected new title to be findable")
	}

	// Old title should NOT be findable (index was replaced)
	got, _ = p.Lookup(database.MediaTypeManga, "Old Title")
	if got != nil {
		t.Error("old title should not be findable after reindex")
	}
}

func TestEnsureTitleIndex_RebuildsForExistingData(t *testing.T) {
	db := tempDB(t)

	// Insert data directly (bypassing pipeline — simulates pre-index database)
	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "eir1"})
	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "eir2"})

	// Create pipeline — ensureTitleIndex should detect missing index and rebuild
	p := pipeline.New(db, pipeline.Options{})

	size, _ := db.TitleIndexSize(database.MediaTypeManga)
	if size < 2 {
		t.Errorf("index size = %d, want >= 2 after EnsureTitleIndex", size)
	}

	// Lookups should work
	got, _ := p.Lookup(database.MediaTypeManga, "One Piece")
	if got == nil {
		t.Fatal("expected lookup to work after EnsureTitleIndex")
	}
}

func TestNormalizeAllTitles_RebuildsIndex(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{
		Type:     database.MediaTypeManga,
		Title:    "Hero\u2019s Party",
		SourceID: "nri1",
	})

	// Index should exist
	size, _ := p.DB().TitleIndexSize(database.MediaTypeManga)
	if size == 0 {
		t.Fatal("expected index to have entries after ingest")
	}

	// NormalizeAllTitles should rebuild index
	p.NormalizeAllTitles()

	// Should still be findable
	got, _ := p.Lookup(database.MediaTypeManga, "Hero's Party")
	if got == nil {
		t.Fatal("expected lookup to work after NormalizeAllTitles rebuilds index")
	}
}

func TestBatchLookup_UsesIndex(t *testing.T) {
	p := tempPipeline(t)

	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "bli1"})
	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "bli2"})
	p.Ingest(&database.Media{Type: database.MediaTypeManga, Title: "Bleach", SourceID: "bli3"})

	titles := []string{"One Piece", "Naruto", "Bleach", "Missing"}
	found, missing := p.BatchLookup(database.MediaTypeManga, titles)

	if len(found) != 3 {
		t.Errorf("found %d, want 3", len(found))
	}
	if len(missing) != 1 {
		t.Errorf("missing %d, want 1", len(missing))
	}
	if missing[0] != "Missing" {
		t.Errorf("missing[0] = %q, want %q", missing[0], "Missing")
	}
}
