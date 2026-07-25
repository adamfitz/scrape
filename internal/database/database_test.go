package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adamfitz/scrape/internal/database"
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

func TestEnsureSchema(t *testing.T) {
	db := tempDB(t)
	if err := db.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
}

func TestInsertAndGetMedia(t *testing.T) {
	db := tempDB(t)

	m := &database.Media{
		Type:     database.MediaTypeManga,
		Title:    "One Piece",
		AltTitle: `{"en":"One Piece","ja":"ワンピース"}`,
		Author:   "Eiichiro Oda",
		SourceID: "a96676e5-8ae2-425e-b549-7f15dd34a6d8",
		Status:   "Ongoing",
		URL:      "https://mangadex.org/title/a96676e5-8ae2-425e-b549-7f15dd34a6d8",
	}

	id, err := db.InsertMedia(m)
	if err != nil {
		t.Fatalf("InsertMedia: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	got, err := db.GetMediaBySourceID("a96676e5-8ae2-425e-b549-7f15dd34a6d8")
	if err != nil {
		t.Fatalf("GetMediaBySourceID: %v", err)
	}
	if got == nil {
		t.Fatal("expected media, got nil")
	}
	if got.Title != "One Piece" {
		t.Errorf("title = %q, want %q", got.Title, "One Piece")
	}
	if got.Author != "Eiichiro Oda" {
		t.Errorf("author = %q, want %q", got.Author, "Eiichiro Oda")
	}
}

func TestGetMediaBySourceID(t *testing.T) {
	db := tempDB(t)

	m := &database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "xyz789"}
	db.InsertMedia(m)

	got, err := db.GetMediaBySourceID("xyz789")
	if err != nil {
		t.Fatalf("GetMediaBySourceID: %v", err)
	}
	if got == nil {
		t.Fatal("expected media, got nil")
	}
	if got.Title != "Naruto" {
		t.Errorf("title = %q, want %q", got.Title, "Naruto")
	}
}

func TestAllMedia(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "a"})
	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "b"})

	all, err := db.AllMedia(database.MediaTypeManga)
	if err != nil {
		t.Fatalf("AllMedia: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("AllMedia returned %d, want 2", len(all))
	}
}

func TestAllMedia_Empty(t *testing.T) {
	db := tempDB(t)

	all, err := db.AllMedia(database.MediaTypeManga)
	if err != nil {
		t.Fatalf("AllMedia: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("AllMedia returned %d, want 0", len(all))
	}
}

func TestStats(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "A", SourceID: "1"})
	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "B", SourceID: "2"})

	stats, err := db.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats["manga"] != 2 {
		t.Errorf("manga count = %d, want 2", stats["manga"])
	}
}

func TestIntegrityCheck(t *testing.T) {
	db := tempDB(t)

	result, err := db.IntegrityCheck()
	if err != nil {
		t.Fatalf("IntegrityCheck: %v", err)
	}
	if result != "ok" {
		t.Errorf("integrity check = %q, want %q", result, "ok")
	}
}

func TestVacuum(t *testing.T) {
	db := tempDB(t)

	if err := db.Vacuum(); err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
}

func TestBackup(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Test", SourceID: "bk1"})

	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := db.Backup(dest); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if _, err := os.Stat(dest); os.IsNotExist(err) {
		t.Fatal("backup file does not exist")
	}

	// Open the backup and verify data
	backupDB, err := database.Open(dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backupDB.Close()

	got, err := backupDB.GetMediaBySourceID("bk1")
	if err != nil {
		t.Fatalf("GetMediaBySourceID on backup: %v", err)
	}
	if got == nil {
		t.Fatal("expected media in backup, got nil")
	}
}

func TestDuplicateSourceID(t *testing.T) {
	db := tempDB(t)

	m1 := &database.Media{Type: database.MediaTypeManga, Title: "A", SourceID: "dup1"}
	m2 := &database.Media{Type: database.MediaTypeManga, Title: "B", SourceID: "dup1"}

	if _, err := db.InsertMedia(m1); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := db.InsertMedia(m2); err == nil {
		t.Fatal("expected error on duplicate source_id, got nil")
	}
}

func TestUpdateMedia(t *testing.T) {
	db := tempDB(t)

	m := &database.Media{Type: database.MediaTypeManga, Title: "Old Title", SourceID: "upd1"}
	id, _ := db.InsertMedia(m)

	m.ID = id
	m.Title = "New Title"
	if err := db.UpdateMedia(m); err != nil {
		t.Fatalf("UpdateMedia: %v", err)
	}

	got, _ := db.GetMediaBySourceID("upd1")
	if got.Title != "New Title" {
		t.Errorf("title = %q, want %q", got.Title, "New Title")
	}
}

func TestUpsertMedia_Insert(t *testing.T) {
	db := tempDB(t)

	m := &database.Media{Type: database.MediaTypeManga, Title: "Upsert Test", SourceID: "up1"}
	id, err := db.UpsertMedia(m)
	if err != nil {
		t.Fatalf("UpsertMedia: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}
}

func TestUpsertMedia_Update(t *testing.T) {
	db := tempDB(t)

	m1 := &database.Media{Type: database.MediaTypeManga, Title: "Original", SourceID: "up2"}
	db.UpsertMedia(m1)

	m2 := &database.Media{Type: database.MediaTypeManga, Title: "Updated", SourceID: "up2"}
	id, err := db.UpsertMedia(m2)
	if err != nil {
		t.Fatalf("UpsertMedia update: %v", err)
	}

	got, _ := db.GetMediaBySourceID("up2")
	if got == nil {
		t.Fatal("expected media, got nil")
	}
	if got.Title != "Updated" {
		t.Errorf("title = %q, want %q", got.Title, "Updated")
	}
	if id != got.ID {
		t.Errorf("returned ID %d != stored ID %d", id, got.ID)
	}
}

func TestGetMediaBySourceID_NotFound(t *testing.T) {
	db := tempDB(t)

	got, err := db.GetMediaBySourceID("nonexistent")
	if err != nil {
		t.Fatalf("GetMediaBySourceID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestAllMedia_MultipleTypes(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Manga 1", SourceID: "m1"})
	db.InsertMedia(&database.Media{Type: database.MediaTypeAnime, Title: "Anime 1", SourceID: "a1"})

	manga, _ := db.AllMedia(database.MediaTypeManga)
	anime, _ := db.AllMedia(database.MediaTypeAnime)

	if len(manga) != 1 {
		t.Errorf("manga count = %d, want 1", len(manga))
	}
	if len(anime) != 1 {
		t.Errorf("anime count = %d, want 1", len(anime))
	}
}

// --- Title Index Tests ---

func TestIndexTitle(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Test", SourceID: "idx1"})

	err := db.IndexTitle(database.MediaTypeManga, 1, "test", "Test")
	if err != nil {
		t.Fatalf("IndexTitle: %v", err)
	}

	size, _ := db.TitleIndexSize(database.MediaTypeManga)
	if size != 1 {
		t.Errorf("index size = %d, want 1", size)
	}
}

func TestQueryTitleIndex(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Naruto", SourceID: "qi1"})
	db.IndexTitle(database.MediaTypeManga, 1, "naruto", "Naruto")

	id, err := db.QueryTitleIndex(database.MediaTypeManga, "naruto")
	if err != nil {
		t.Fatalf("QueryTitleIndex: %v", err)
	}
	if id != 1 {
		t.Errorf("media_id = %d, want 1", id)
	}

	// Not found
	id, err = db.QueryTitleIndex(database.MediaTypeManga, "bleach")
	if err != nil {
		t.Fatalf("QueryTitleIndex: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for missing title, got %d", id)
	}
}

func TestIndexTitles_ReplacesEntries(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "Test", SourceID: "ir1"})

	// Index with 2 titles
	titles := map[string]string{"test": "Test", "test alt": "Test Alt"}
	err := db.IndexTitles(database.MediaTypeManga, 1, titles)
	if err != nil {
		t.Fatalf("IndexTitles: %v", err)
	}

	size, _ := db.TitleIndexSize(database.MediaTypeManga)
	if size != 2 {
		t.Errorf("index size = %d, want 2", size)
	}

	// Replace with 1 title
	titles2 := map[string]string{"new test": "New Test"}
	db.IndexTitles(database.MediaTypeManga, 1, titles2)

	size, _ = db.TitleIndexSize(database.MediaTypeManga)
	if size != 1 {
		t.Errorf("index size after replace = %d, want 1", size)
	}

	id, _ := db.QueryTitleIndex(database.MediaTypeManga, "new test")
	if id != 1 {
		t.Errorf("media_id = %d, want 1", id)
	}
}

func TestClearTitleIndex(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "A", SourceID: "cl1"})
	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "B", SourceID: "cl2"})
	db.IndexTitle(database.MediaTypeManga, 1, "a", "A")
	db.IndexTitle(database.MediaTypeManga, 2, "b", "B")

	db.ClearTitleIndex(database.MediaTypeManga, 1)

	size, _ := db.TitleIndexSize(database.MediaTypeManga)
	if size != 1 {
		t.Errorf("index size = %d, want 1", size)
	}

	id, _ := db.QueryTitleIndex(database.MediaTypeManga, "b")
	if id != 2 {
		t.Errorf("remaining entry media_id = %d, want 2", id)
	}
}

func TestBatchQueryTitleIndex(t *testing.T) {
	db := tempDB(t)

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "A", SourceID: "bq1"})
	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "B", SourceID: "bq2"})
	db.IndexTitle(database.MediaTypeManga, 1, "a", "A")
	db.IndexTitle(database.MediaTypeManga, 2, "b", "B")

	matches, err := db.BatchQueryTitleIndex(database.MediaTypeManga, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("BatchQueryTitleIndex: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("matches = %d, want 2", len(matches))
	}
	if matches["a"] != 1 {
		t.Errorf("matches[a] = %d, want 1", matches["a"])
	}
	if matches["b"] != 2 {
		t.Errorf("matches[b] = %d, want 2", matches["b"])
	}
}

func TestGetMediaByID(t *testing.T) {
	db := tempDB(t)

	id, _ := db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "One Piece", SourceID: "gb1"})

	got, err := db.GetMediaByID(database.MediaTypeManga, id)
	if err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected media, got nil")
	}
	if got.Title != "One Piece" {
		t.Errorf("title = %q, want %q", got.Title, "One Piece")
	}
}

func TestGetMediaByID_NotFound(t *testing.T) {
	db := tempDB(t)

	got, err := db.GetMediaByID(database.MediaTypeManga, 99999)
	if err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestTitleIndexSize(t *testing.T) {
	db := tempDB(t)

	size, _ := db.TitleIndexSize(database.MediaTypeManga)
	if size != 0 {
		t.Errorf("empty index size = %d, want 0", size)
	}

	db.InsertMedia(&database.Media{Type: database.MediaTypeManga, Title: "A", SourceID: "ts1"})
	db.IndexTitle(database.MediaTypeManga, 1, "a", "A")
	db.IndexTitle(database.MediaTypeManga, 1, "a alt", "A Alt")

	size, _ = db.TitleIndexSize(database.MediaTypeManga)
	if size != 2 {
		t.Errorf("index size = %d, want 2", size)
	}

	total, _ := db.TitleIndexSizeTotal()
	if total != 2 {
		t.Errorf("total index size = %d, want 2", total)
	}
}
