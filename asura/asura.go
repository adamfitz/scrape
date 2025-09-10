package asura

import (
	"context"
	//"time"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	//"strconv"
	"os"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// chapterImage holds URL and order
type chapterImage struct {
	Order int
	URL   string
}

// ExtractChapterLinksFromURL fetches the series page and returns all valid chapter URLs
func ExtractChapterLinksFromURL(seriesURL string) ([]string, error) {
	log.Printf("[asura - ExtractChapterLinksFromURL] Fetching series page: %s\n", seriesURL)

	resp, err := http.Get(seriesURL)
	if err != nil {
		return nil, fmt.Errorf("[asura - ExtractChapterLinksFromURL] failed to fetch series page: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[asura - ExtractChapterLinksFromURL] HTTP status code: %d\n", resp.StatusCode)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("[asura - ExtractChapterLinksFromURL] failed to fetch series page: status code %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[asura - ExtractChapterLinksFromURL] failed to parse HTML: %w", err)
	}

	chapterURLs := make(map[string]struct{})
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}
		href = strings.TrimSpace(href)
		log.Printf("[asura - ExtractChapterLinksFromURL] Found href: %s\n", href)

		// Keep any link that contains "/chapter/"
		if strings.Contains(href, "/chapter/") {
			// Ensure full URL
			if !strings.HasPrefix(href, "http") {
				href = "https://asuracomic.net/series/" + strings.TrimPrefix(href, "/")
			}
			log.Printf("[asura - ExtractChapterLinksFromURL] Matched chapter URL: %s\n", href)
			chapterURLs[href] = struct{}{}
		}
	})

	// Convert map to slice and sort descending (latest first)
	var urls []string
	for u := range chapterURLs {
		urls = append(urls, u)
	}
	sort.Slice(urls, func(i, j int) bool {
		return urls[i] > urls[j] // descending
	})

	log.Printf("[asura - ExtractChapterLinksFromURL] Total chapters found: %d\n", len(urls))
	return urls, nil
}

// ChapterFilenames normalizes chapter URLs into consistent filenames
func ChapterFilenames(urls []string) map[string]string {
	result := make(map[string]string)

	// Regex to extract chapter number with optional subchapter (dot or dash)
	re := regexp.MustCompile(`chapter/([\d]+(?:[.-]\d+)?)`)

	for _, u := range urls {
		matches := re.FindStringSubmatch(u)
		if len(matches) < 2 {
			continue // skip URLs without a chapter number
		}

		chNum := matches[1] // e.g., "43", "43.4", "54-4"

		mainNum := chNum
		part := ""

		// Normalize any subchapter/part to use dot
		if strings.ContainsAny(chNum, ".-") {
			if strings.Contains(chNum, ".") {
				parts := strings.SplitN(chNum, ".", 2)
				mainNum = parts[0]
				part = "." + parts[1]
			} else if strings.Contains(chNum, "-") {
				parts := strings.SplitN(chNum, "-", 2)
				mainNum = parts[0]
				part = "." + parts[1] // <-- convert dash to dot
			}
		}

		// Pad main number to 3 digits
		filename := fmt.Sprintf("ch%03s%s.cbz", mainNum, part)
		result[filename] = u
	}

	return result
}

// GetChapterImageURLs fetches chapter images with detailed logging and proper script parsing.
func GetChapterImageURLs(chapterURL string) ([]chapterImage, error) {
	log.Printf("[asura - GetChapterImageURLs] Starting fetch for: %s", chapterURL)

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var html string
	log.Printf("[asura - GetChapterImageURLs] Navigating to page...")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(chapterURL),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}
	log.Printf("[asura - GetChapterImageURLs] Navigation complete. HTML length: %d", len(html))

	// Optionally save page to /tmp for inspection
	tmpFile, err := os.CreateTemp("/tmp", "asura-chapter-*.html")
	if err == nil {
		tmpFile.WriteString(html)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())
		log.Printf("[asura - GetChapterImageURLs] Saved page to %s for inspection", tmpFile.Name())
	}

	// Extract <script> tags
	scripts := extractScriptsFromHTML(html)
	log.Printf("[asura - GetChapterImageURLs] Total <script> tags found: %d", len(scripts))

	var images []chapterImage
	for i, script := range scripts {
		log.Printf("[asura - GetChapterImageURLs] Parsing script %d content length: %d", i, len(script))
		matches := extractImageURLsFromScript(script) // implement regex/json parsing
		log.Printf("[asura - GetChapterImageURLs] Script %d matches found: %d", i, len(matches))

		for j, url := range matches {
			images = append(images, chapterImage{
				Order: j + 1, // ensure sequential ordering
				URL:   url,
			})
		}
	}

	// Sort by Order just in case
	sort.Slice(images, func(i, j int) bool {
		return images[i].Order < images[j].Order
	})

	log.Printf("[asura - GetChapterImageURLs] Total images extracted and sorted: %d", len(images))
	return images, nil
}

/*
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
*/

// Returns the content of all <script> tags in the HTML
func extractScriptsFromHTML(html string) []string {
	var scripts []string
	re := regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`)
	matches := re.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		scripts = append(scripts, m[1])
	}
	return scripts
}

// Parses a single script block for image URLs
func extractImageURLsFromScript(script string) []string {
	var urls []string
	// Look for the Asura CDN images ending in optimized.webp or .jpg/.png
	re := regexp.MustCompile(`https://gg\.asuracomic\.net/storage/media/[0-9]+/conversions/[0-9a-fA-F-]+-optimized\.(webp|jpg|png)`)
	matches := re.FindAllString(script, -1)
	for _, m := range matches {
		urls = append(urls, m)
	}
	return urls
}
