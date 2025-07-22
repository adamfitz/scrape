package kunmanga

import (
	"archive/zip"
	"errors"
	"fmt"
	"github.com/gocolly/colly"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Download image from URL and save to disk
func downloadKunMangaImage(imgURL, outputPath string, referer string) error {
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Required headers to avoid 403
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", referer)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("GET %s => %d\n", imgURL, resp.StatusCode)

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// mangaName is the name of the manga from the url eg:
// From: https://kunmanga.com/manga/ugly-complex/
// the mangaName will be the string "ugly-complex"
func KunMangaChapterUrls(mangaName string) []string {

	baseUrl := "https://kunmanga.com/manga"
	c := colly.NewCollector(
		colly.AllowedDomains("kunmanga.com"),
	)

	var chapterLinks []string

	// Select all <a> elements under the chapter list
	c.OnHTML("ul.main.version-chap li.wp-manga-chapter > a", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		chapterLinks = append(chapterLinks, link)
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("\nVisiting:", r.URL.String())
	})

	// build the url to visit
	err := c.Visit(baseUrl + "/" + mangaName + "/")
	if err != nil {
		log.Fatal(err)
	}

	return chapterLinks
}

// DownloadKunMangaChapters downloads chapter images to temp, zips to CBZ, cleans up.
// Returns error on failure.
// DownloadKunMangaChapters downloads chapter images to temp, zips to CBZ, cleans up.
// Saves CBZ as ch<num>.cbz in current directory. Returns error on failure.
func DownloadKunMangaChapters(url string, chapterNumber int) error {
	tempDir := filepath.Join(os.TempDir(), "chapter-dl")
	chapterSlug := filepath.Base(strings.Trim(url, "/"))
	chapterTempDir := filepath.Join(tempDir, chapterSlug)

	log.Printf("[INFO] Starting download for chapter %s", chapterSlug)

	err := os.MkdirAll(chapterTempDir, 0755)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to create temp directory %s: %w", chapterTempDir, err)
	}

	c := colly.NewCollector(
		colly.AllowedDomains("kunmanga.com"),
	)
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"

	var imageURLs []string

	c.OnHTML("div.reading-content img", func(e *colly.HTMLElement) {
		imgURL := strings.TrimSpace(e.Attr("src"))
		if imgURL != "" {
			imageURLs = append(imageURLs, imgURL)
		}
	})

	c.OnRequest(func(r *colly.Request) {
		log.Printf("[INFO] Visiting %s", r.URL.String())
	})

	err = c.Visit(url)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to visit page %s: %w", url, err)
	}

	if len(imageURLs) == 0 {
		log.Printf("[WARN] No images found for chapter %s", chapterSlug)
		return errors.New("no images found on chapter page")
	}

	// Download images with retry logic
	for i, imgURL := range imageURLs {
		outputPath := filepath.Join(chapterTempDir, fmt.Sprintf("%03d%s", i+1, filepath.Ext(imgURL)))
		var lastErr error

		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf("[INFO] Downloading image %d/%d: %s (attempt %d)", i+1, len(imageURLs), imgURL, attempt)
			lastErr = downloadKunMangaImage(imgURL, outputPath, url)
			if lastErr == nil {
				log.Printf("[INFO] Successfully downloaded %s", imgURL)
				break
			}
			log.Printf("[WARN] Failed to download %s on attempt %d: %v", imgURL, attempt, lastErr)
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

// parseChapterNumber extracts the number from strings like "chapter-18" or "chapter-18-5"
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

// CBZExists checks if ch<num>.cbz already exists in current directory.
// Returns true if exists, false otherwise. Returns error if unexpected.
func CBZExists(chapterNumber int) (bool, error) {
	var cbzName string
	if chapterNumber < 10 {
		cbzName = fmt.Sprintf("ch%02d.cbz", chapterNumber)
	} else {
		cbzName = fmt.Sprintf("ch%d.cbz", chapterNumber)
	}
	cbzPath := filepath.Join(".", cbzName)

	_, err := os.Stat(cbzPath)
	if err == nil {
		// File exists
		return true, nil
	}
	if os.IsNotExist(err) {
		// File does not exist
		return false, nil
	}
	// Other unexpected error
	return false, err
}
