// Package pipeline composes normalisation with database operations.
// All title normalisation for storage and lookup lives here — the database
// layer is a pure data store with no normalisation logic.
//
// The pipeline is a synchronous, single-call library. Each method executes
// a complete operation and returns a result. The pipeline does not manage
// concurrency — the calling application is responsible for goroutines,
// mutexes, and lifecycle management if needed.
package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamfitz/scrape/internal/database"
	"github.com/adamfitz/scrape/internal/mangadex"
	"github.com/adamfitz/scrape/internal/normalize"
	"github.com/adamfitz/scrape/internal/ratelimit"
)

// --- Error types ---

// ErrorKind identifies the category of a pipeline error.
type ErrorKind int

const (
	ErrNotFound         ErrorKind = iota // title not found in DB or API
	ErrAPIMatchRejected                  // API returned results but none matched the query
	ErrAPIFailed                         // MangaDex API returned an error
	ErrIngestFailed                      // failed to write to database
	ErrDatabase                          // database read/write error
)

var errorKindNames = [...]string{
	ErrNotFound:         "not found",
	ErrAPIMatchRejected: "no matching result",
	ErrAPIFailed:        "API error",
	ErrIngestFailed:     "ingest failed",
	ErrDatabase:         "database error",
}

// PipelineError is a structured error returned by pipeline methods.
// Callers can use errors.As to extract the kind and handle it appropriately.
type PipelineError struct {
	Kind    ErrorKind
	Message string
	Err     error // underlying error, if any
}

func (e *PipelineError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", errorKindNames[e.Kind], e.Message, e.Err)
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", errorKindNames[e.Kind], e.Message)
	}
	return errorKindNames[e.Kind]
}

func (e *PipelineError) Unwrap() error {
	return e.Err
}

// --- Pipeline types ---

// Source indicates where a lookup result came from.
type Source string

const (
	SourceDB    Source = "db"
	SourceAPI   Source = "api"
	SourceFuzzy Source = "fuzzy"
)

// AltTitleData is the structured format for multi-language title variants.
type AltTitleData struct {
	Primary map[string]string   `json:"primary"`
	Alts    []map[string]string `json:"alts"`
}

// Candidate represents an API match that needs user disambiguation.
type Candidate struct {
	Title     string
	SourceID  string
	Language  string
	AltTitles AltTitleData
	URL       string
	Status    string
	DescEn    string
}

// LookupResult is the result of a lookup operation.
type LookupResult struct {
	Media      *database.Media // non-nil when found/ingested
	Source     Source
	Query      string
	Error      string      // non-empty on error/not-found
	Candidates []Candidate // non-empty when multiple API matches need disambiguation
}

// LookupOptions configures a lookup operation.
type LookupOptions struct {
	LocalOnly bool // skip API calls
	Limit     int  // max API results to consider (default 5)
}

// BatchLookupOptions configures a batch lookup operation.
type BatchLookupOptions struct {
	LocalOnly bool
	Limit     int
}

// Options configures a Pipeline instance.
type Options struct {
	RateLimit float64 // requests per second for MangaDex API (default 4)
}

// --- Pipeline struct ---

// Pipeline is the single entry point for all data operations.
// Create one at application startup via New and reuse it for the lifetime
// of the process. All methods are safe to call from a single goroutine.
type Pipeline struct {
	normaliser *normalize.Normalizer
	db         *database.DB
}

// New creates a Pipeline with the given database and options.
func New(db *database.DB, opts Options) *Pipeline {
	p := &Pipeline{
		normaliser: normalize.New(normalize.NormalizationConfig{}),
		db:         db,
	}
	p.ensureTitleIndex()
	return p
}

// ensureTitleIndex checks if the title index is empty for any media type
// and rebuilds it. This handles databases created before the index existed.
func (p *Pipeline) ensureTitleIndex() {
	for _, mt := range database.AllMediaTypes() {
		size, err := p.db.TitleIndexSize(mt)
		if err != nil {
			continue
		}
		if size > 0 {
			continue
		}
		// Index is empty — check if the table has data
		all, err := p.db.AllMedia(mt)
		if err != nil || len(all) == 0 {
			continue
		}
		// Rebuild index for this media type
		for i := range all {
			p.reindexMedia(mt, all[i].ID, &all[i])
		}
	}
}

// DB returns the pipeline's underlying database connection.
func (p *Pipeline) DB() *database.DB {
	return p.db
}

// Normalizer returns the pipeline's normaliser instance for external callers.
func (p *Pipeline) Normalizer() *normalize.Normalizer {
	return p.normaliser
}

// --- Internal helpers ---

func parseAltTitles(raw string) AltTitleData {
	if raw == "" {
		return AltTitleData{}
	}
	var structured AltTitleData
	if err := json.Unmarshal([]byte(raw), &structured); err == nil && structured.Primary != nil {
		return structured
	}
	var flat map[string]string
	if err := json.Unmarshal([]byte(raw), &flat); err == nil {
		return AltTitleData{Primary: flat}
	}
	return AltTitleData{}
}

// ParseAltTitles parses stored alt_title JSON into AltTitleData.
func ParseAltTitles(raw string) AltTitleData {
	return parseAltTitles(raw)
}

// allTitles returns the primary title plus all alt titles for a media record.
func allTitles(m *database.Media) []string {
	titles := []string{m.Title}
	if m.AltTitle == "" {
		return titles
	}
	data := parseAltTitles(m.AltTitle)
	for _, t := range data.Primary {
		if t != "" && t != m.Title {
			titles = append(titles, t)
		}
	}
	for _, alt := range data.Alts {
		for _, t := range alt {
			if t != "" && t != m.Title {
				titles = append(titles, t)
			}
		}
	}
	return titles
}

// normalizeAllTitles applies FoldUnicode to the title and alt_title for storage.
func normalizeAllTitles(m *database.Media) (string, string) {
	norm := normalize.FoldUnicode(m.Title)
	altNorm := normalize.NormalizeAllTitlesJSON(m.AltTitle)
	return norm, altNorm
}

// reindexMedia updates the title index for a single media record.
func (p *Pipeline) reindexMedia(mediaType database.MediaType, mediaID int64, m *database.Media) error {
	titles := allTitles(m)
	index := make(map[string]string, len(titles))
	for _, raw := range titles {
		norm := p.normaliser.MustNormalize(raw)
		if norm != "" {
			index[norm] = raw
		}
	}
	return p.db.IndexTitles(mediaType, mediaID, index)
}

var apostropheReplacer = strings.NewReplacer(
	"'", "", "\u2018", "", "\u2019", "", "\u02BC", "", "\u2032", "", "\uFF07", "",
	"\"", "", "\u201C", "", "\u201D", "", "\u00AB", "", "\u00BB", "", "\u2033", "", "\uFF02", "",
)

func queryMatchesAnyTitle(normalizedQuery string, result mangadex.MangaResult, n *normalize.Normalizer) bool {
	queryNoSpace := strings.ReplaceAll(normalizedQuery, " ", "")
	check := func(rawTitle string) bool {
		normalized := n.MustNormalize(rawTitle)
		if strings.Contains(normalized, normalizedQuery) {
			return true
		}
		if queryNoSpace != "" && strings.Contains(strings.ReplaceAll(normalized, " ", ""), queryNoSpace) {
			return true
		}
		stripped := apostropheReplacer.Replace(rawTitle)
		if stripped != rawTitle {
			normalized2 := n.MustNormalize(stripped)
			if strings.Contains(normalized2, normalizedQuery) {
				return true
			}
		}
		return false
	}
	for _, t := range result.Attributes.Title {
		if t != "" && check(t) {
			return true
		}
	}
	for _, alt := range result.Attributes.AltTitles {
		for _, t := range alt {
			if t != "" && check(t) {
				return true
			}
		}
	}

	// 4th strategy: fuzzy similarity
	if normalizedQuery != "" {
		fuzzyCheck := func(rawTitle string) bool {
			normalized := n.MustNormalize(rawTitle)
			if normalize.Similarity(normalizedQuery, normalized) >= 0.85 {
				return true
			}
			return false
		}
		for _, t := range result.Attributes.Title {
			if t != "" && fuzzyCheck(t) {
				return true
			}
		}
		for _, alt := range result.Attributes.AltTitles {
			for _, t := range alt {
				if t != "" && fuzzyCheck(t) {
					return true
				}
			}
		}
	}
	return false
}

// extractTitles pulls primary and alt titles from a MangaDex API result.
func extractTitles(r mangadex.MangaResult) AltTitleData {
	return AltTitleData{
		Primary: r.Attributes.Title,
		Alts:    r.Attributes.AltTitles,
	}
}

// extractCandidate wraps a MangaDex result into a Candidate for user disambiguation.
func extractCandidate(r mangadex.MangaResult, query string) Candidate {
	titles := extractTitles(r)
	b, _ := json.Marshal(titles)
	title := TitleOrFallback(string(b), "en", query)
	return Candidate{
		Title:     title,
		SourceID:  r.ID,
		Language:  r.Attributes.OriginalLanguage,
		AltTitles: titles,
		URL:       fmt.Sprintf("https://mangadex.org/title/%s", r.ID),
		Status:    r.Attributes.Status,
		DescEn:    r.Attributes.Description["en"],
	}
}

// ingestResult converts a MangaDex API result into a Media record and stores it.
func (p *Pipeline) ingestResult(mediaType database.MediaType, query string, r mangadex.MangaResult) (*LookupResult, error) {
	titles := extractTitles(r)
	b, _ := json.Marshal(titles)
	title := TitleOrFallback(string(b), "en", query)

	media := &database.Media{
		Type:        mediaType,
		Title:       title,
		AltTitle:    string(b),
		SourceID:    r.ID,
		URL:         fmt.Sprintf("https://mangadex.org/title/%s", r.ID),
		Language:    r.Attributes.OriginalLanguage,
		Status:      r.Attributes.Status,
		Description: r.Attributes.Description["en"],
	}
	if _, err := p.Ingest(media); err != nil {
		return nil, &PipelineError{Kind: ErrIngestFailed, Message: title, Err: err}
	}
	return &LookupResult{Media: media, Source: SourceAPI, Query: query}, nil
}

// --- Public API ---

// Lookup searches for a media record by title in the local database.
// Uses the pre-computed title index for O(log N) lookups.
func (p *Pipeline) Lookup(mediaType database.MediaType, title string) (*database.Media, error) {
	queryNorm := p.normaliser.MustNormalize(title)

	mediaID, err := p.db.QueryTitleIndex(mediaType, queryNorm)
	if err != nil {
		return nil, &PipelineError{Kind: ErrDatabase, Message: "title index lookup", Err: err}
	}
	if mediaID == 0 {
		return nil, nil
	}

	media, err := p.db.GetMediaByID(mediaType, mediaID)
	if err != nil {
		return nil, &PipelineError{Kind: ErrDatabase, Message: "fetch media by id", Err: err}
	}
	return media, nil
}

// LookupAPI performs a full lookup: local DB first, then MangaDex API.
func (p *Pipeline) LookupAPI(mediaType database.MediaType, query string, opts LookupOptions) (*LookupResult, error) {
	media, err := p.Lookup(mediaType, query)
	if err != nil {
		return nil, err
	}
	if media != nil {
		return &LookupResult{Media: media, Source: SourceDB, Query: query}, nil
	}

	// Tier 2: Fuzzy scan against local DB
	queryNorm := p.normaliser.MustNormalize(query)
	fuzzyCandidates := p.fuzzyScan(mediaType, queryNorm, 0.85)
	if len(fuzzyCandidates) == 1 {
		return &LookupResult{Media: fuzzyCandidates[0].Media, Source: SourceFuzzy, Query: query}, nil
	}
	if len(fuzzyCandidates) > 1 {
		cands := make([]Candidate, len(fuzzyCandidates))
		for i, fc := range fuzzyCandidates {
			cands[i] = Candidate{
				Title:    fc.Title,
				SourceID: fc.Media.SourceID,
				Language: fc.Media.Language,
				URL:      fc.Media.URL,
				Status:   fc.Media.Status,
				DescEn:   fc.Media.Description,
			}
		}
		return &LookupResult{Source: SourceFuzzy, Query: query, Candidates: cands}, nil
	}

	if opts.LocalOnly {
		return &LookupResult{Source: SourceDB, Query: query, Error: "not found locally"}, nil
	}

	// Tier 3: MangaDex API
	rl := ratelimit.New(4)
	defer rl.Stop()
	client := mangadex.New(rl)

	apiQuery := QueryForAPI(query)
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}

	results, err := client.SearchManga(apiQuery, limit)
	if err != nil {
		return nil, &PipelineError{Kind: ErrAPIFailed, Message: fmt.Sprintf("search %q", query), Err: err}
	}

	if len(results) == 0 {
		return &LookupResult{Source: SourceAPI, Query: query, Error: "no results found on MangaDex"}, nil
	}

	var matches []mangadex.MangaResult
	for _, r := range results {
		if queryMatchesAnyTitle(apiQuery, r, p.normaliser) {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return &LookupResult{Source: SourceAPI, Query: query, Error: "no matching result on MangaDex"}, nil
	}

	if len(matches) == 1 {
		return p.ingestResult(mediaType, query, matches[0])
	}

	candidates := make([]Candidate, len(matches))
	for i, r := range matches {
		candidates[i] = extractCandidate(r, query)
	}
	return &LookupResult{Source: SourceAPI, Query: query, Candidates: candidates}, nil
}

// IngestCandidate ingests a user-selected API candidate into the database.
func (p *Pipeline) IngestCandidate(mediaType database.MediaType, c Candidate) (*database.Media, error) {
	altB, _ := json.Marshal(c.AltTitles)
	media := &database.Media{
		Type:        mediaType,
		Title:       c.Title,
		AltTitle:    string(altB),
		SourceID:    c.SourceID,
		URL:         c.URL,
		Language:    c.Language,
		Status:      c.Status,
		Description: c.DescEn,
	}
	if _, err := p.Ingest(media); err != nil {
		return nil, err
	}
	return media, nil
}

// Ingest normalises a media record and inserts or updates it in the database.
func (p *Pipeline) Ingest(m *database.Media) (int64, error) {
	m.Title, m.AltTitle = normalizeAllTitles(m)
	id, err := p.db.UpsertMedia(m)
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		id = m.ID
	}
	if err := p.reindexMedia(m.Type, id, m); err != nil {
		return id, err
	}
	return id, nil
}

// BatchLookup checks multiple titles against the local database using the index.
func (p *Pipeline) BatchLookup(mediaType database.MediaType, titles []string) (map[string]*database.Media, []string) {
	if len(titles) == 0 {
		return nil, nil
	}

	normalised := make([]string, len(titles))
	normToOriginal := make(map[string]string, len(titles))
	for i, t := range titles {
		n := p.normaliser.MustNormalize(t)
		normalised[i] = n
		normToOriginal[n] = t
	}

	matches, err := p.db.BatchQueryTitleIndex(mediaType, normalised)
	if err != nil {
		return nil, titles
	}

	found := make(map[string]*database.Media)
	missing := make([]string, 0, len(titles))

	// Collect unique media IDs to fetch
	idSet := make(map[int64]bool)
	for _, id := range matches {
		idSet[id] = true
	}
	mediaByID := make(map[int64]*database.Media)
	for id := range idSet {
		m, err := p.db.GetMediaByID(mediaType, id)
		if err == nil && m != nil {
			mediaByID[id] = m
		}
	}

	for _, n := range normalised {
		orig := normToOriginal[n]
		if id, ok := matches[n]; ok {
			if m, ok := mediaByID[id]; ok {
				found[orig] = m
				continue
			}
		}
		missing = append(missing, orig)
	}

	return found, missing
}

// BatchLookupStream performs batch lookups and sends results to the returned
// channel. The channel is closed when all titles have been processed.
func (p *Pipeline) BatchLookupStream(mediaType database.MediaType, titles []string, opts BatchLookupOptions) <-chan LookupResult {
	ch := make(chan LookupResult)

	go func() {
		defer close(ch)

		if len(titles) == 0 {
			return
		}

		found, missing := p.BatchLookup(mediaType, titles)

		for _, title := range titles {
			if m, ok := found[title]; ok {
				ch <- LookupResult{Media: m, Source: SourceDB, Query: title}
			}
		}

		if len(missing) == 0 {
			return
		}

		// Tier 2: Fuzzy scan against local DB before hitting API
		var stillMissing []string
		for _, title := range missing {
			queryNorm := p.normaliser.MustNormalize(title)
			candidates := p.fuzzyScan(mediaType, queryNorm, 0.85)
			if len(candidates) == 0 {
				stillMissing = append(stillMissing, title)
				continue
			}
			if len(candidates) == 1 {
				// Single fuzzy match — link query to existing record
				_ = p.LinkQuery(mediaType, candidates[0].Media.ID, title)
				ch <- LookupResult{Media: candidates[0].Media, Source: SourceFuzzy, Query: title}
				continue
			}
			// Multiple fuzzy candidates — emit for disambiguation
			cands := make([]Candidate, len(candidates))
			for i, fc := range candidates {
				cands[i] = Candidate{
					Title:    fc.Title,
					SourceID: fc.Media.SourceID,
					Language: fc.Media.Language,
					URL:      fc.Media.URL,
					Status:   fc.Media.Status,
					DescEn:   fc.Media.Description,
				}
			}
			ch <- LookupResult{Source: SourceFuzzy, Query: title, Candidates: cands}
		}

		if opts.LocalOnly {
			return
		}

		rl := ratelimit.New(4)
		defer rl.Stop()
		client := mangadex.New(rl)
		limit := opts.Limit
		if limit <= 0 {
			limit = 5
		}

		for _, title := range stillMissing {
			apiQuery := QueryForAPI(title)
			apiResults, err := client.SearchManga(apiQuery, limit)
			if err != nil {
				ch <- LookupResult{Source: SourceAPI, Query: title, Error: fmt.Sprintf("MangaDex search failed: %v", err)}
				continue
			}

			if len(apiResults) == 0 {
				ch <- LookupResult{Source: SourceAPI, Query: title, Error: "no results found on MangaDex"}
				continue
			}

			var best mangadex.MangaResult
			var matched bool
			bestLen := -1
			for _, r := range apiResults {
				if queryMatchesAnyTitle(apiQuery, r, p.normaliser) {
					t := extractTitles(r)
					b, _ := json.Marshal(t)
					matchedTitle := TitleOrFallback(string(b), "en", title)
					normLen := len(QueryForAPI(matchedTitle))
					if bestLen < 0 || normLen < bestLen {
						best = r
						bestLen = normLen
						matched = true
					}
				}
			}

			if !matched {
				ch <- LookupResult{Source: SourceAPI, Query: title, Error: "no matching result on MangaDex"}
				continue
			}

			result, err := p.ingestResult(mediaType, title, best)
			if err != nil {
				ch <- LookupResult{Source: SourceAPI, Query: title, Error: err.Error()}
				continue
			}
			ch <- *result
		}
	}()

	return ch
}

// FuzzyCandidate represents a local DB record found by fuzzy matching.
type FuzzyCandidate struct {
	Media *database.Media
	Score float64
	Title string // the matched title string
}

// FuzzyLookupResult is the result of a local fuzzy scan.
type FuzzyLookupResult struct {
	Exact     *database.Media  // exact index match (preferred)
	Fuzzy     []FuzzyCandidate // candidates above fuzzy threshold
	Query     string
	QueryNorm string
}

// fuzzyScan scans allMedia for fuzzy matches against queryNorm.
// Returns candidates sorted by score descending.
func (p *Pipeline) fuzzyScan(mediaType database.MediaType, queryNorm string, threshold float64) []FuzzyCandidate {
	all, err := p.db.AllMedia(mediaType)
	if err != nil {
		return nil
	}
	var candidates []FuzzyCandidate
	for i := range all {
		titles := allTitles(&all[i])
		for _, raw := range titles {
			norm := p.normaliser.MustNormalize(raw)
			score := normalize.Similarity(queryNorm, norm)
			if score >= threshold {
				candidates = append(candidates, FuzzyCandidate{
					Media: &all[i],
					Score: score,
					Title: raw,
				})
				break // one match per media record is enough
			}
		}
	}
	// Sort by score descending (simple bubble — small N)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	return candidates
}

// FuzzyLookup performs a local-only lookup with fuzzy fallback.
// Returns the exact match if found, otherwise returns fuzzy candidates.
func (p *Pipeline) FuzzyLookup(mediaType database.MediaType, query string, threshold float64) *FuzzyLookupResult {
	queryNorm := p.normaliser.MustNormalize(query)
	result := &FuzzyLookupResult{Query: query, QueryNorm: queryNorm}

	// Try exact index match first
	media, _ := p.Lookup(mediaType, query)
	if media != nil {
		result.Exact = media
		return result
	}

	// Fuzzy scan
	result.Fuzzy = p.fuzzyScan(mediaType, queryNorm, threshold)
	return result
}

// LinkQuery links a fuzzy-matched title back to an existing media record by
// inserting a secondary entry in the title index. This ensures subsequent
// exact-match lookups find the record without re-scanning.
func (p *Pipeline) LinkQuery(mediaType database.MediaType, mediaID int64, query string) error {
	queryNorm := p.normaliser.MustNormalize(query)
	if queryNorm == "" {
		return nil
	}
	index := map[string]string{queryNorm: query}
	return p.db.IndexTitles(mediaType, mediaID, index)
}

// NormalizeStats holds the result of a normalisation pass.
type NormalizeStats struct {
	Updated int64 // records whose title/alt_title column changed
	Indexed int64 // records re-indexed in the title index
}

// NormalizeAllTitles re-normalises every title across all media types and
// rebuilds the title index. Safe to run repeatedly (idempotent).
func (p *Pipeline) NormalizeAllTitles() (NormalizeStats, error) {
	var stats NormalizeStats

	for _, mt := range database.AllMediaTypes() {
		updated, indexed, err := p.normalizeTitlesForType(mt)
		if err != nil {
			return stats, err
		}
		stats.Updated += updated
		stats.Indexed += indexed
	}

	return stats, nil
}

// normalizeTitlesForType re-normalises titles and rebuilds the index for one media type.
// Returns (updated, indexed, error) where updated is records whose title changed
// and indexed is the total records re-indexed.
func (p *Pipeline) normalizeTitlesForType(mediaType database.MediaType) (int64, int64, error) {
	all, err := p.db.AllMedia(mediaType)
	if err != nil {
		return 0, 0, &PipelineError{Kind: ErrDatabase, Message: fmt.Sprintf("fetch all %s", mediaType.TableName()), Err: err}
	}

	// Clear the index for this media type — we'll rebuild as we go
	if err := p.db.RebuildTitleIndex(mediaType); err != nil {
		return 0, 0, &PipelineError{Kind: ErrDatabase, Message: fmt.Sprintf("rebuild index for %s", mediaType.TableName()), Err: err}
	}

	var updated, indexed int64
	for i := range all {
		newTitle := normalize.FoldUnicode(all[i].Title)
		newAlt := normalize.NormalizeAllTitlesJSON(all[i].AltTitle)

		if newTitle == all[i].Title && newAlt == all[i].AltTitle {
			// Title unchanged, but still index it
			if err := p.reindexMedia(mediaType, all[i].ID, &all[i]); err != nil {
				return updated, indexed, &PipelineError{Kind: ErrDatabase, Message: fmt.Sprintf("reindex %s %d", mediaType.TableName(), all[i].ID), Err: err}
			}
			indexed++
			continue
		}

		all[i].Title = newTitle
		all[i].AltTitle = newAlt
		if err := p.db.UpdateMedia(&all[i]); err != nil {
			return updated, indexed, &PipelineError{Kind: ErrDatabase, Message: fmt.Sprintf("update %s %d", mediaType.TableName(), all[i].ID), Err: err}
		}
		if err := p.reindexMedia(mediaType, all[i].ID, &all[i]); err != nil {
			return updated, indexed, &PipelineError{Kind: ErrDatabase, Message: fmt.Sprintf("reindex %s %d", mediaType.TableName(), all[i].ID), Err: err}
		}
		updated++
		indexed++
	}
	return updated, indexed, nil
}

// QueryForAPI normalises a title for use as a MangaDex API search query.
func QueryForAPI(title string) string {
	n := normalize.New(normalize.NormalizationConfig{})
	q, _ := n.Normalize(title)
	return q
}

// Deduplicate removes duplicate titles using normalised forms as keys.
// Used to deduplicate input files before batch processing.
func Deduplicate(titles []string) []string {
	n := normalize.New(normalize.NormalizationConfig{})
	seen := make(map[string]bool)
	var result []string
	for _, t := range titles {
		key := n.MustNormalize(t)
		if key == "" {
			continue
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, t)
		}
	}
	return result
}

// TitleOrFallback returns the title in the requested language, falling back
// to the first available language or the provided fallback string.
func TitleOrFallback(altTitleJSON, lang, fallback string) string {
	data := parseAltTitles(altTitleJSON)

	if t, ok := data.Primary[lang]; ok && t != "" {
		return t
	}
	for _, t := range data.Primary {
		if t != "" {
			return t
		}
	}
	return fallback
}
