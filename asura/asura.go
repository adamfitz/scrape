package asura

import (
	"context"
	//"time"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

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

// Fetchesetches all image URLs from a chapter page (unsorted)
func GetRawChapterImageURLs(chapterURL string) ([]string, error) {
	log.Printf("[asura - GetRawChapterImageURLs] Starting fetch for: %s", chapterURL)

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var html string
	log.Printf("[asura - GetRawChapterImageURLs] Navigating to page...")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(chapterURL),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return nil, fmt.Errorf("[asura - GetRawChapterImageURLs] navigation failed: %w", err)
	}
	log.Printf("[asura - GetRawChapterImageURLs] Navigation complete. HTML length: %d", len(html))

	// Optionally save page to /tmp for inspection
	tmpFile, err := os.CreateTemp("/tmp", "asura-chapter-*.html")
	if err == nil {
		tmpFile.WriteString(html)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())
		log.Printf("[asura - GetRawChapterImageURLs] Saved page to %s for inspection", tmpFile.Name())
	}

	// Extract <script> tags
	scripts := extractScriptsFromHTML(html)
	log.Printf("[asura - GetRawChapterImageURLs] Total <script> tags found: %d", len(scripts))

	var urls []string
	for i, script := range scripts {
		matches := extractImageURLsFromScript(script)
		log.Printf("[asura - GetRawChapterImageURLs] Script %d matches found: %d", i, len(matches))
		urls = append(urls, matches...)
	}

	log.Printf("[asura - GetRawChapterImageURLs] Total raw image URLs extracted: %d", len(urls))
	return urls, nil
}

// FilterImageURLs filters URLs to only those with filenames like xx-optimized.webp
func FilterImageURLs(urls []string) []string {
	var filtered []string
	re := regexp.MustCompile(`(\d{1,3})-optimized\.webp$`)

	for _, u := range urls {
		if re.MatchString(u) {
			filtered = append(filtered, u)
		} else {
			log.Printf("[asura - FilterImageURLs] Ignored URL (does not match pattern): %s", u)
		}
	}

	log.Printf("[asura - FilterImageURLs] Filtered image URLs count: %d", len(filtered))
	return filtered
}

// BuildChapterImages converts filtered URLs into a sorted slice of chapterImage
func BuildChapterImages(urls []string) []chapterImage {
	type temp struct {
		order int
		url   string
	}

	var tmpList []temp
	re := regexp.MustCompile(`(\d{1,3})-optimized\.webp$`)

	for _, u := range urls {
		m := re.FindStringSubmatch(u)
		if len(m) == 2 {
			num, err := strconv.Atoi(m[1])
			if err != nil {
				log.Printf("[asura - BuildChapterImages] Failed to parse number from URL %s: %v", u, err)
				continue
			}
			tmpList = append(tmpList, temp{order: num, url: u})
			log.Printf("[asura - BuildChapterImages] Added URL %s with order %d", u, num)
		} else {
			log.Printf("[asura - BuildChapterImages] Skipping URL (regex did not match): %s", u)
		}
	}

	// Sort by the numeric prefix
	sort.Slice(tmpList, func(i, j int) bool {
		return tmpList[i].order < tmpList[j].order
	})

	// Build final slice
	images := make([]chapterImage, len(tmpList))
	for i, t := range tmpList {
		images[i] = chapterImage{
			Order: i + 1,
			URL:   t.url,
		}
		log.Printf("[asura - BuildChapterImages] Final ChapterImage [%d]: %s", images[i].Order, images[i].URL)
	}

	log.Printf("[asura - BuildChapterImages] Total chapter images built: %d", len(images))
	return images
}

// GetSortedChapterImages is now updated to deduplicate URLs first
func GetSortedChapterImages(chapterURL string) ([]chapterImage, error) {
	rawURLs, err := GetRawChapterImageURLs(chapterURL)
	if err != nil {
		return nil, err
	}

	filtered := FilterImageURLs(rawURLs)
	deduped := DeduplicateURLs(filtered)
	chapterImages := BuildChapterImages(deduped)

	log.Printf("[asura - GetSortedChapterImages] Total sorted chapter images after deduplication: %d", len(chapterImages))
	return chapterImages, nil
}

// DeduplicateURLs removes duplicate URLs from a slice while preserving order
func DeduplicateURLs(urls []string) []string {
	log.Printf("[asura - DeduplicateURLs] Starting deduplication. Total URLs: %d", len(urls))
	seen := make(map[string]struct{})
	var deduped []string

	for _, u := range urls {
		if _, ok := seen[u]; ok {
			log.Printf("[asura - DeduplicateURLs] Skipping duplicate URL: %s", u)
			continue
		}
		seen[u] = struct{}{}
		deduped = append(deduped, u)
	}

	log.Printf("[asura - DeduplicateURLs] Deduplication complete. Remaining URLs: %d", len(deduped))
	return deduped
}

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
