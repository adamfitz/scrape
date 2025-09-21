package kunmanga

import (
	"archive/zip"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"scrape/cfproxy"
	"strconv"
	"strings"
	"time"
)

// KunMangaChapterUrls fetches chapter URLs using regular HTTP client
func KunMangaChapterUrls(shortName string) []string {
	return getChapterURLsWithClient(shortName, http.DefaultClient)
}

// KunMangaChapterUrlsWithProxy fetches chapter URLs using CF proxy
func KunMangaChapterUrlsWithProxy(shortName string, proxyPort int) []string {
	client := cfproxy.NewProxiedClient(proxyPort)
	return getChapterURLsWithClient(shortName, client)
}

// getChapterURLsWithClient fetches chapter URLs using the provided HTTP client
func getChapterURLsWithClient(shortName string, client *http.Client) []string {
	mangaURL := fmt.Sprintf("https://kunmanga.com/manga/%s", shortName)

	log.Printf("[INFO] Fetching manga page: %s", mangaURL)

	req, err := http.NewRequest("GET", mangaURL, nil)
	if err != nil {
		log.Printf("[ERROR] Failed to create request: %v", err)
		return nil
	}

	// Set browser-like headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch manga page: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[ERROR] Manga page returned status %d", resp.StatusCode)
		return nil
	}

	// Parse HTML to extract chapter URLs
	chapterURLs, err := extractChapterURLsFromHTML(resp.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to parse chapter URLs: %v", err)
		return nil
	}

	log.Printf("[INFO] Found %d chapters", len(chapterURLs))
	return chapterURLs
}

// DownloadKunMangaChapters downloads a chapter using regular HTTP client
func DownloadKunMangaChapters(url string, chapterNumber int) error {
	return downloadChapterWithClient(url, chapterNumber, http.DefaultClient)
}

// DownloadKunMangaChaptersWithProxy downloads a chapter using CF proxy
func DownloadKunMangaChaptersWithProxy(url string, chapterNumber int, proxyPort int) error {
	client := cfproxy.NewProxiedClient(proxyPort)
	return downloadChapterWithClient(url, chapterNumber, client)
}

// downloadChapterWithClient is the core download function that accepts any HTTP client
func downloadChapterWithClient(url string, chapterNumber int, client *http.Client) error {
	tempDir := filepath.Join(os.TempDir(), "chapter-dl")
	chapterSlug := filepath.Base(strings.Trim(url, "/"))
	chapterTempDir := filepath.Join(tempDir, chapterSlug)

	log.Printf("[INFO] Starting download for chapter %s", chapterSlug)

	err := os.MkdirAll(chapterTempDir, 0755)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to create temp directory %s: %w", chapterTempDir, err)
	}

	// Get the chapter page content
	log.Printf("[INFO] Fetching chapter page: %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to create request for %s: %w", url, err)
	}

	// Set browser-like headers for chapter page
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to fetch chapter page %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("[ERROR] Chapter page returned status %d for %s", resp.StatusCode, url)
	}

	// Parse HTML to extract image URLs
	imageURLs, err := extractImageURLsFromHTML(resp.Body)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to parse HTML for %s: %w", url, err)
	}

	if len(imageURLs) == 0 {
		log.Printf("[WARN] No images found for chapter %s", chapterSlug)
		return errors.New("no images found on chapter page")
	}

	log.Printf("[INFO] Found %d images for chapter %s", len(imageURLs), chapterSlug)

	// Download images with retry logic
	for i, imgURL := range imageURLs {
		// Handle relative URLs
		if strings.HasPrefix(imgURL, "/") {
			imgURL = "https://kunmanga.com" + imgURL
		} else if !strings.HasPrefix(imgURL, "http") {
			imgURL = "https://" + imgURL
		}

		outputPath := filepath.Join(chapterTempDir, fmt.Sprintf("%03d%s", i+1, getImageExtension(imgURL)))
		var lastErr error

		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf("[INFO] Downloading image %d/%d: %s (attempt %d)", i+1, len(imageURLs), imgURL, attempt)
			lastErr = downloadImageWithClient(imgURL, outputPath, url, client)
			if lastErr == nil {
				log.Printf("[INFO] Successfully downloaded image %d", i+1)
				break
			}
			log.Printf("[WARN] Failed to download image %d on attempt %d: %v", i+1, attempt, lastErr)
			time.Sleep(time.Duration(attempt*2) * time.Second) // Exponential backoff
		}

		if lastErr != nil {
			log.Printf("[ERROR] Giving up on image %s after 3 attempts: %v", imgURL, lastErr)
			return fmt.Errorf("failed to download image %s: %w", imgURL, lastErr)
		}
	}

	// Create CBZ file as ch<num>.cbz in current directory
	outputDir := "."
	var cbzName string
	if chapterNumber < 10 {
		cbzName = fmt.Sprintf("ch%02d.cbz", chapterNumber)
	} else {
		cbzName = fmt.Sprintf("ch%d.cbz", chapterNumber)
	}
	cbzPath := filepath.Join(outputDir, cbzName)

	err = createCBZFromDir(cbzPath, chapterTempDir)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to create CBZ %s: %w", cbzPath, err)
	}
	log.Printf("[INFO] Created CBZ archive %s", cbzPath)

	// Cleanup temp files
	err = os.RemoveAll(chapterTempDir)
	if err != nil {
		log.Printf("[WARN] Failed to remove temp directory %s: %v", chapterTempDir, err)
	} else {
		log.Printf("[INFO] Removed temp directory %s", chapterTempDir)
	}

	log.Printf("[INFO] Finished download for chapter %s", chapterSlug)
	return nil
}

// downloadImageWithClient downloads a single image using the provided HTTP client
func downloadImageWithClient(imgURL, outputPath, referer string, client *http.Client) error {
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to mimic browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("image download returned status %d", resp.StatusCode)
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Copy image data
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	return nil
}

// extractChapterURLsFromHTML parses the manga page to find chapter links
func extractChapterURLsFromHTML(htmlContent io.Reader) ([]string, error) {
	doc, err := html.Parse(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var chapterURLs []string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			// Look for chapter links - typically in elements with "wp-manga-chapter" class
			if isChapterLink(n) {
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						chapterURL := strings.TrimSpace(attr.Val)
						if chapterURL != "" {
							// Convert relative URLs to absolute
							if strings.HasPrefix(chapterURL, "/") {
								chapterURL = "https://kunmanga.com" + chapterURL
							}
							chapterURLs = append(chapterURLs, chapterURL)
						}
						break
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	return chapterURLs, nil
}

// extractImageURLsFromHTML parses HTML content to find manga image URLs
func extractImageURLsFromHTML(htmlContent io.Reader) ([]string, error) {
	doc, err := html.Parse(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var imageURLs []string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			// Look for images in the reading content
			if hasClass(n, "wp-manga-chapter-img") ||
				isInReadingContent(n) {
				for _, attr := range n.Attr {
					if attr.Key == "src" || attr.Key == "data-src" {
						imgURL := strings.TrimSpace(attr.Val)
						if imgURL != "" && !strings.Contains(imgURL, "loading") && !strings.Contains(imgURL, "placeholder") {
							imageURLs = append(imageURLs, imgURL)
						}
						break
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	return imageURLs, nil
}

// createCBZFromDir zips all files from srcDir into a zip file at cbzPath
func createCBZFromDir(cbzPath, srcDir string) error {
	cbzFile, err := os.Create(cbzPath)
	if err != nil {
		return fmt.Errorf("failed to create CBZ file: %w", err)
	}
	defer cbzFile.Close()

	zipWriter := zip.NewWriter(cbzFile)
	defer zipWriter.Close()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read temp dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // skip subdirs if any
		}

		filePath := filepath.Join(srcDir, entry.Name())
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file for zipping: %w", err)
		}

		w, err := zipWriter.Create(entry.Name())
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to create zip entry: %w", err)
		}

		_, err = io.Copy(w, file)
		file.Close()
		if err != nil {
			return fmt.Errorf("failed to write file to zip: %w", err)
		}
	}

	return nil
}

// ParseChapterNumber extracts the number from strings like "chapter-18" or "chapter-18-5"
func ParseChapterNumber(slug string) int {
	slug = strings.ToLower(slug)
	// Find first digit sequence after "chapter-"
	prefix := "chapter-"
	if !strings.HasPrefix(slug, prefix) {
		return 0
	}
	numPart := slug[len(prefix):]

	// Sometimes chapters can have subnumbers like "18-5", we parse up to first non-digit
	numStr := ""
	for _, r := range numPart {
		if r >= '0' && r <= '9' {
			numStr += string(r)
		} else {
			break
		}
	}

	if numStr == "" {
		return 0
	}

	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return n
}

// Helper function to identify chapter links
func isChapterLink(n *html.Node) bool {
	// Check if this is a chapter link based on common patterns
	for _, attr := range n.Attr {
		if attr.Key == "class" && strings.Contains(attr.Val, "wp-manga-chapter") {
			return true
		}
		if attr.Key == "href" && strings.Contains(attr.Val, "/chapter/") {
			return true
		}
	}

	// Check parent elements for chapter listing context
	parent := n.Parent
	for parent != nil && parent.Type == html.ElementNode {
		for _, attr := range parent.Attr {
			if attr.Key == "class" {
				if strings.Contains(attr.Val, "wp-manga-chapter") ||
					strings.Contains(attr.Val, "chapter-list") ||
					strings.Contains(attr.Val, "listing-chapters") {
					return true
				}
			}
		}
		parent = parent.Parent
	}

	return false
}

// Helper function to check if node has a specific class
func hasClass(n *html.Node, className string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			return strings.Contains(attr.Val, className)
		}
	}
	return false
}

// Helper function to check if node is within reading content
func isInReadingContent(n *html.Node) bool {
	current := n.Parent
	for current != nil {
		if current.Type == html.ElementNode && current.Data == "div" {
			for _, attr := range current.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "reading-content") {
					return true
				}
			}
		}
		current = current.Parent
	}
	return false
}

// Helper function to get proper image extension
func getImageExtension(imgURL string) string {
	// Extract extension from URL, default to .jpg
	parts := strings.Split(imgURL, ".")
	if len(parts) > 1 {
		ext := "." + parts[len(parts)-1]
		// Remove query parameters
		if strings.Contains(ext, "?") {
			ext = strings.Split(ext, "?")[0]
		}
		// Validate common image extensions
		validExts := []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}
		for _, validExt := range validExts {
			if strings.EqualFold(ext, validExt) {
				return ext
			}
		}
	}
	return ".jpg" // Default extension
}
