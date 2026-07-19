# mangadex-api-client Specification

## Purpose

Query the MangaDex API to look up manga by title and extract multi-language title variants (Japanese, Japanese Hepburn, Korean, Chinese, English).

## Requirements

### Requirement: Manga search by title

The client SHALL query `GET https://api.mangadex.org/manga` with a `title` query parameter and return structured results.

#### Scenario: Successful search

- GIVEN the title `"One Piece"`
- WHEN the client searches MangaDex
- THEN the response SHALL contain manga results with ID, title map, and alt-titles
- AND the response SHALL include the `originalLanguage` field

#### Scenario: No results

- GIVEN a title that matches no manga on MangaDex
- WHEN the client searches MangaDex
- THEN the result set SHALL be empty
- AND no error SHALL be returned

#### Scenario: API error

- GIVEN MangaDex returns a non-200 status code
- WHEN the client processes the response
- THEN an error SHALL be returned with the status code and response body

### Requirement: User-Agent header

The client SHALL include a `User-Agent` header in all requests. The value SHALL be `"scrape/1.0"` or a user-configured string.

#### Scenario: Default User-Agent

- GIVEN no custom User-Agent configuration
- WHEN a request is made
- THEN the `User-Agent` header SHALL be `"scrape/1.0"`

### Requirement: Title extraction (structured)

The client SHALL extract titles as structured `AltTitleData` containing the primary title map and the full alt titles array. The primary title and alt titles SHALL be stored separately — alt titles SHALL NOT override the primary title for the same language.

#### Scenario: Extract primary and alt titles

- GIVEN a manga with primary `{"en": "One Piece"}` and alt titles `[{"ja": "ワンピース"}, {"en": "One Piece"}]`
- WHEN `ExtractTitles` is called
- THEN the result SHALL have `Primary: {"en": "One Piece"}` and `Alts: [{"ja": "ワンピース"}, {"en": "One Piece"}]`
- AND the primary English title SHALL be `"One Piece"` (not overridden by alt titles)

#### Scenario: Anthology with sub-story alt titles

- GIVEN a manga with primary `{"en": "Anthology Title"}` and alt titles including sub-story names like `[{"en": "Story A"}, {"en": "Story B"}]`
- WHEN `ExtractTitles` is called
- THEN the primary title SHALL remain `"Anthology Title"`
- AND all alt titles SHALL be preserved in the `Alts` array for lookup matching

### Requirement: HTTP timeout

The client SHALL enforce a 10-second HTTP timeout for all requests.

#### Scenario: Slow response

- GIVEN MangaDex takes longer than 10 seconds to respond
- WHEN the timeout fires
- THEN the request SHALL fail with a timeout error

### Requirement: Configurable search limit

The client SHALL accept a `limit` parameter (1-100, default 5) to control how many results are returned per search.

#### Scenario: Limited results

- GIVEN a search with `limit=3`
- WHEN results are returned
- THEN no more than 3 manga results SHALL be in the response

## Response Schema

```go
type MangaResult struct {
    ID         string          `json:"id"`
    Attributes MangaAttributes `json:"attributes"`
}

type MangaAttributes struct {
    Title            map[string]string   `json:"title"`
    AltTitles        []map[string]string `json:"altTitles"`
    OriginalLanguage string              `json:"originalLanguage"`
    Description      map[string]string   `json:"description"`
    Status           string              `json:"status"`
    Tags             []Tag               `json:"tags"`
}

type AltTitleData struct {
    Primary map[string]string   `json:"primary"`
    Alts    []map[string]string `json:"alts"`
}

type LookupResult struct {
    Query         string        `json:"query"`
    MangaDexID    string        `json:"mangadex_id"`
    Title         string        `json:"title"`
    AltTitles     AltTitleData  `json:"alt_titles"`
    Author        string        `json:"author,omitempty"`
    Description   string        `json:"description,omitempty"`
    CoverURL      string        `json:"cover_url,omitempty"`
    URL           string        `json:"url"`
    Status        string        `json:"status"`
    OriginalLang  string        `json:"original_language"`
    Error         string        `json:"error,omitempty"`
}
```

## Implementation Notes

- Port the MangaDex client from `/home/adam/projects/resolve/internal/mangadex/client.go`
- `ExtractTitles` returns `AltTitleData` (primary + alts array), NOT a flat map
- Primary title always wins — alt titles are stored alongside for lookup matching
- Add author extraction via `includes` parameter (`?includes[]=author`)
- Add cover art extraction via `includes[]=cover_art`
- The client MUST NOT be called without going through the rate limiter
- API queries are normalized before sending (handles periods, underscores, separators)
