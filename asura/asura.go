package asura

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"scrape/parser"

	"github.com/PuerkitoBio/goquery"
	"github.com/chai2010/webp"
	"github.com/chromedp/chromedp"
)

// chapterImage holds URL and order
type chapterImage struct {
	Order int
	URL   string
}

// download manga chapters
func DownloadChapters(url string) {

	// step zero call clean up func
	var tempDirList []string
	parser.CleanupTempDirs(&tempDirList)

	// Step 1: grab the list of chapter urls from the provided link
	chapters, chapterListError := extractChapterLinksFromURL(url)
	if chapterListError != nil {
		log.Fatalf("[asura - extractChapterLinksFromURL] error, %v", chapterListError)
	}
	// Step 2: create the chapter map with filename as key and url as value
	chapterMap := chapterFilenames(chapters)

	// Step 3: get existing CBZ files so their download can be skipped
	existing, err := parser.GetDownloadedCBZ(".")
	if err != nil {
		log.Fatalf("Failed to read current directory: %v", err)
	}

	// Step 4: delete any downloaded (existing) chapters from the map
	// (the map key is the filename and value is true or false for some reason)
	for downloadedChapter := range existing {
		delete(chapterMap, downloadedChapter)
	}

	// Step 5: sort the map keys to get ordered chapter list
	chapterList, keySortError := parser.SortKeys(chapterMap)
	if chapterListError != nil {
		log.Fatalf("[asura - parser.SortKeys] error, %v", keySortError)
	}

	// Step 6: download each chapter
	for _, chapterName := range chapterList {
		fmt.Printf("%s\t%s\n", chapterName, chapterMap[chapterName])
		chapterImages, sortChImgsErr := sortedChapterImages(chapterMap[chapterName])
		if sortChImgsErr != nil {
			log.Fatalf("[asura - sortedChapterImages] Failed to get and sort images: %v", sortChImgsErr)
		}

		// create the temp directory (chapterName and random string create the temp dir name)
		tempDir, err := parser.CreateTempDir(chapterName)
		if err != nil {
			log.Printf("[asura - parser.CreateTempDir] could not create temp directory: %s", tempDir)
		}

		// append to the list of temp directories that need to be removed once the download func finishes or
		// crashes/hard exits
		tempDirList = append(tempDirList, tempDir)
		log.Printf("Created tempdir: %s", tempDir)

		// download chapter images to temp directory
		for _, image := range chapterImages {
			if err := downloadChapterImage(image, tempDir); err != nil {
				log.Printf("[asura - DownloadChapterImage] Error: %v", err)
			}
		}

		// create the cbz file *(chapterName == filename)
		parser.CreateCbzFromDir(tempDir, chapterName)
		fmt.Printf("Downloaded: %s\n", chapterName)
	}
}

// Fetches the series page and returns all valid chapter URLs
func extractChapterLinksFromURL(seriesURL string) ([]string, error) {
	log.Printf("[asura - extractChapterLinksFromURL] Fetching series page: %s\n", seriesURL)

	resp, err := http.Get(seriesURL)
	if err != nil {
		return nil, fmt.Errorf("[asura - extractChapterLinksFromURL] failed to fetch series page: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[asura - extractChapterLinksFromURL] HTTP status code: %d\n", resp.StatusCode)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("[asura - extractChapterLinksFromURL] failed to fetch series page: status code %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[asura - extractChapterLinksFromURL] failed to parse HTML: %w", err)
	}

	chapterURLs := make(map[string]struct{})
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}
		href = strings.TrimSpace(href)
		log.Printf("[asura - extractChapterLinksFromURL] Found href: %s\n", href)

		// Keep any link that contains "/chapter/"
		if strings.Contains(href, "/chapter/") {
			// Ensure full URL
			if !strings.HasPrefix(href, "http") {
				href = "https://asuracomic.net/series/" + strings.TrimPrefix(href, "/")
			}
			log.Printf("[asura - extractChapterLinksFromURL] Matched chapter URL: %s\n", href)
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

	log.Printf("[asura - extractChapterLinksFromURL] Total chapters found: %d\n", len(urls))
	return urls, nil
}

// chapterFilenames normalizes chapter URLs into consistent filenames
func chapterFilenames(urls []string) map[string]string {
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

// Fetches all image URLs from a chapter page (unsorted)
func rawChapterImageUrls(chapterURL string) ([]string, error) {
	log.Printf("[asura - rawChapterImageUrls] Starting fetch for: %s", chapterURL)

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var html string
	log.Printf("[asura - rawChapterImageUrls] Navigating to page...")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(chapterURL),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return nil, fmt.Errorf("[asura - rawChapterImageUrls] navigation failed: %w", err)
	}
	log.Printf("[asura - rawChapterImageUrls] Navigation complete. HTML length: %d", len(html))

	// Optionally save page to /tmp for inspection
	tmpFile, err := os.CreateTemp("/tmp", "asura-chapter-*.html")
	if err == nil {
		tmpFile.WriteString(html)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())
		log.Printf("[asura - rawChapterImageUrls] Saved page to %s for inspection", tmpFile.Name())
	}

	// Extract <script> tags
	scripts := extractScriptsFromHTML(html)
	log.Printf("[asura - rawChapterImageUrls] Total <script> tags found: %d", len(scripts))

	var urls []string
	for i, script := range scripts {
		matches := extractImageURLsFromScript(script)
		log.Printf("[asura - rawChapterImageUrls] Script %d matches found: %d", i, len(matches))
		urls = append(urls, matches...)
	}

	log.Printf("[asura - rawChapterImageUrls] Total raw image URLs extracted: %d", len(urls))
	return urls, nil
}

// filterImageURLs filters URLs to only those with filenames like xx-optimized.webp
func filterImageURLs(urls []string) []string {
	var filtered []string
	re := regexp.MustCompile(`(\d{1,3})-optimized\.webp$`)

	for _, u := range urls {
		if re.MatchString(u) {
			filtered = append(filtered, u)
		} else {
			log.Printf("[asura - filterImageURLs] Ignored URL (does not match pattern): %s", u)
		}
	}

	log.Printf("[asura - filterImageURLs] Filtered image URLs count: %d", len(filtered))
	return filtered
}

// buildChapterImages converts filtered URLs into a sorted slice of chapterImage
func buildChapterImages(urls []string) []chapterImage {
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
				log.Printf("[asura - buildChapterImages] Failed to parse number from URL %s: %v", u, err)
				continue
			}
			tmpList = append(tmpList, temp{order: num, url: u})
			log.Printf("[asura - buildChapterImages] Added URL %s with order %d", u, num)
		} else {
			log.Printf("[asura - buildChapterImages] Skipping URL (regex did not match): %s", u)
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
		log.Printf("[asura - buildChapterImages] Final ChapterImage [%d]: %s", images[i].Order, images[i].URL)
	}

	log.Printf("[asura - buildChapterImages] Total chapter images built: %d", len(images))
	return images
}

// Deduplicates and sorts the chapter URLs
func sortedChapterImages(chapterURL string) ([]chapterImage, error) {
	rawURLs, err := rawChapterImageUrls(chapterURL)
	if err != nil {
		return nil, err
	}

	filtered := filterImageURLs(rawURLs)
	deduped := deduplicateURLs(filtered)
	chapterImages := buildChapterImages(deduped)

	log.Printf("[asura - sortedChapterImages] Total sorted chapter images after deduplication: %d", len(chapterImages))
	return chapterImages, nil
}

// deduplicateURLs removes duplicate URLs from a slice while preserving order
func deduplicateURLs(urls []string) []string {
	log.Printf("[asura - deduplicateURLs] Starting deduplication. Total URLs: %d", len(urls))
	seen := make(map[string]struct{})
	var deduped []string

	for _, u := range urls {
		if _, ok := seen[u]; ok {
			log.Printf("[asura - deduplicateURLs] Skipping duplicate URL: %s", u)
			continue
		}
		seen[u] = struct{}{}
		deduped = append(deduped, u)
	}

	log.Printf("[asura - deduplicateURLs] Deduplication complete. Remaining URLs: %d", len(deduped))
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
	// Look for the Asura CDN images ending in optimized.webp or .jpg/.png
	re := regexp.MustCompile(`https://gg\.asuracomic\.net/storage/media/[0-9]+/conversions/[0-9a-fA-F-]+-optimized\.(webp|jpg|png)`)
	return re.FindAllString(script, -1)
}

/*
// sorts chapterImage struct slice
func SortChapterImages(chapterImages []chapterImage) {
	sort.Slice(chapterImages, func(i, j int) bool {
		return chapterImages[i].Order < chapterImages[j].Order
	})
}
*/

// Downloads and converts a chapterImage (with Order + URL)
// into a JPG file saved under targetDir, naming files sequentially using Order.
func downloadChapterImage(img chapterImage, targetDir string) error {
	resp, err := http.Get(img.URL)
	if err != nil {
		return fmt.Errorf("failed to download image (Order %d): %w", img.Order, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad response status for %s: %s", img.URL, resp.Status)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read image data for %s: %w", img.URL, err)
	}

	format, err := parser.DetectImageFormat(imgBytes)
	if err != nil {
		return fmt.Errorf("failed to detect image format for %s: %w", img.URL, err)
	}

	// Generate filename from the Order field
	paddedFileName := fmt.Sprintf("%03d.jpg", img.Order)
	outputFile := filepath.Join(targetDir, paddedFileName)

	// If already JPEG, just save raw bytes directly
	if format == "jpeg" {
		err = os.WriteFile(outputFile, imgBytes, 0644)
		if err != nil {
			return fmt.Errorf("failed to save jpeg image for %s: %w", img.URL, err)
		}
		log.Printf("[asura - DownloadChapterImage] Saved JPEG directly: %s", outputFile)
		return nil
	}

	// Decode image according to detected format
	var decoded image.Image
	switch format {
	case "png", "gif":
		decoded, _, err = image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode %s: %w", img.URL, err)
		}
	case "webp":
		decoded, err = webp.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode webp %s: %w", img.URL, err)
		}
	default:
		return fmt.Errorf("unsupported image format for %s: %s", img.URL, format)
	}

	// Convert and save as JPG
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file for %s: %w", img.URL, err)
	}
	defer outFile.Close()

	opts := jpeg.Options{Quality: 90}
	err = jpeg.Encode(outFile, decoded, &opts)
	if err != nil {
		return fmt.Errorf("failed to encode jpeg for %s: %w", img.URL, err)
	}

	log.Printf("[asura - DownloadChapterImage] Wrote %s (Order %d)", outputFile, img.Order)
	return nil
}
