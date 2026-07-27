// Package mangadex queries the MangaDex API to look up manga by title and
// return multi-language title variants.
package mangadex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adamfitz/scrape/internal/ratelimit"
)

const baseURL = "https://api.mangadex.org"

// Client interacts with the MangaDex API with rate limiting.
type Client struct {
	http      *http.Client
	rateLimit *ratelimit.RateLimiter
	userAgent string
}

// New creates a MangaDex API client with the given rate limiter.
func New(rl *ratelimit.RateLimiter) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
		rateLimit: rl,
		userAgent: "scrape/1.0",
	}
}

// MangaResult represents a single manga from the MangaDex API.
type MangaResult struct {
	ID         string          `json:"id"`
	Attributes MangaAttributes `json:"attributes"`
}

// MangaAttributes holds the detailed attributes of a manga.
type MangaAttributes struct {
	Title            map[string]string   `json:"title"`
	AltTitles        []map[string]string `json:"altTitles"`
	OriginalLanguage string              `json:"originalLanguage"`
	Description      map[string]string   `json:"description"`
	Status           string              `json:"status"`
	Tags             []Tag               `json:"tags"`
}

// Tag represents a genre or theme tag.
type Tag struct {
	ID         string        `json:"id"`
	Attributes TagAttributes `json:"attributes"`
}

// TagAttributes holds tag metadata.
type TagAttributes struct {
	Name map[string]string `json:"name"`
}

// Relationship represents an included related entity (author, artist, cover_art).
type Relationship struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type searchResponse struct {
	Data   []MangaResult `json:"data"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

type authorResponse struct {
	Data []Relationship `json:"data"`
}

// SearchManga queries MangaDex with rate limiting.
func (c *Client) SearchManga(title string, limit int) ([]MangaResult, error) {
	if err := c.rateLimit.Wait(context.Background()); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 5
	}

	u, _ := url.Parse(baseURL + "/manga")
	q := url.Values{}
	q.Set("title", title)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("includes[]", "author")
	q.Set("includes[]", "cover_art")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", title, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 1
		if h := resp.Header.Get("X-RateLimit-Retry-After"); h != "" {
			if v, err := strconv.Atoi(h); err == nil {
				retryAfter = v
			}
		}
		time.Sleep(time.Duration(retryAfter) * time.Second)
		return c.SearchManga(title, limit)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mangadex API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return sr.Data, nil
}

// AltTitleData is the structured format stored in the alt_title column.
type AltTitleData struct {
	Primary map[string]string   `json:"primary"`
	Alts    []map[string]string `json:"alts"`
}

// ExtractTitles pulls primary and alt titles from a MangaDex API result.
func ExtractTitles(m MangaResult) AltTitleData {
	return AltTitleData{
		Primary: m.Attributes.Title,
		Alts:    m.Attributes.AltTitles,
	}
}

// ExtractAuthorName finds the author name from a manga's relationships array.
func ExtractAuthorName(relationships []Relationship) string {
	for _, rel := range relationships {
		if rel.Type == "author" {
			if name, ok := rel.Attributes["name"].(string); ok {
				return name
			}
		}
	}
	return ""
}

// ExtractCoverURL builds the cover image URL from cover_art relationships.
func ExtractCoverURL(mangaID string, relationships []Relationship) string {
	for _, rel := range relationships {
		if rel.Type == "cover_art" {
			if fileName, ok := rel.Attributes["fileName"].(string); ok {
				return fmt.Sprintf("https://uploads.mangadex.org/covers/%s/%s.512.jpg", mangaID, fileName)
			}
		}
	}
	return ""
}
