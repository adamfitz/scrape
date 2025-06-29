package manhuaus

import (
	"archive/zip"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"bytes"
	"github.com/gocolly/colly"
	"golang.org/x/image/webp" // Add support for decoding webp
	"scrape/webClient"
)

// ChapterInfo holds chapter URL and number
type ChapterInfo struct {
	URL        string
	ChapterNum int
}

// Download chapter images and create cbz file
func DownloadChaper(chapterURL string) error {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "manga_chapter")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	c := colly.NewCollector()

	var imgURLs []string
	var chapterValue string

	// Extract chapter value
	c.OnHTML("input#wp-manga-current-chap", func(e *colly.HTMLElement) {
		val := strings.TrimSpace(e.Attr("value"))
		if val != "" {
			chapterValue = val
		}
	})

	c.OnHTML("div.reading-content img", func(e *colly.HTMLElement) {
		src := strings.TrimSpace(e.Attr("data-src"))
		if src != "" {
			imgURLs = append(imgURLs, src)
		}
	})

	err = c.Visit(chapterURL)
	if err != nil {
		return err
	}

	if len(imgURLs) == 0 {
		return fmt.Errorf("no images found")
	}
	if chapterValue == "" {
		return fmt.Errorf("chapter value not found")
	}

	fmt.Println("Found", len(imgURLs), "images. Downloading and converting to JPG...")

	ticker := time.NewTicker(1500 * time.Millisecond) // 1 request every 1.5 sec
	defer ticker.Stop()

	client := webClient.NewHTTPClient()

	for i, url := range imgURLs {
		<-ticker.C

		req, err := webClient.NewImageRequest(url, chapterURL)
		if err != nil {
			return fmt.Errorf("failed to create request for %s: %v", url, err)
		}

		bodyBytes, err := webClient.FetchImageBytes(client, req)
		if err != nil {
			return err
		}

		img, err := webp.Decode(bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to decode webp image %s: %v", url, err)
		}

		fileName := fmt.Sprintf("page-%03d.jpg", i+1)
		filePath := filepath.Join(tmpDir, fileName)

		outFile, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %v", filePath, err)
		}

		err = jpeg.Encode(outFile, img, &jpeg.Options{Quality: 90})
		outFile.Close()
		if err != nil {
			return fmt.Errorf("failed to save image %s: %v", url, err)
		}

		fmt.Println("Saved:", fileName)
	}

	// Build CBZ file name
	cbzFileName := fmt.Sprintf("ch%s.cbz", strings.TrimPrefix(chapterValue, "chapter-"))
	cbzFile, err := os.Create(cbzFileName)
	if err != nil {
		return err
	}
	defer cbzFile.Close()

	zipWriter := zip.NewWriter(cbzFile)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		filePath := filepath.Join(tmpDir, file.Name())
		fileToZip, err := os.Open(filePath)
		if err != nil {
			return err
		}

		wr, err := zipWriter.Create(file.Name())
		if err != nil {
			fileToZip.Close()
			return err
		}

		_, err = io.Copy(wr, fileToZip)
		fileToZip.Close()
		if err != nil {
			return err
		}
	}

	err = zipWriter.Close()
	if err != nil {
		return err
	}

	fmt.Println("CBZ file created:", cbzFileName)
	return nil
}

// Retrieves list of chapter URLs
func ChapterURLs(mangaURL string) ([]string, error) {
	var chapterURLs []string

	c := colly.NewCollector()

	// The chapter links are inside: <li class="wp-manga-chapter"><a href="...">...</a></li>
	c.OnHTML("li.wp-manga-chapter a", func(e *colly.HTMLElement) {
		url := e.Attr("href")
		if url != "" {
			chapterURLs = append(chapterURLs, url)
		}
	})

	err := c.Visit(mangaURL)
	if err != nil {
		return nil, err
	}

	if len(chapterURLs) == 0 {
		return nil, fmt.Errorf("no chapter URLs found at %s", mangaURL)
	}

	return chapterURLs, nil
}

// ExtractChapterNumber extracts the chapter number the gin URL
func ExtractChapterNumber(url string) (string, error) {
	re := regexp.MustCompile(`chapter-(\d+)`)
	match := re.FindStringSubmatch(url)
	if len(match) >= 2 {
		return match[1], nil
	}
	return "", fmt.Errorf("chapter number not found in URL")
}

// SortAndFilterChapters sorts and filters chapter URLs by optional start and end chapter numbers.
func SortAndFilterChapters(urls []string, start, end int) ([]string, error) {
	var chapters []ChapterInfo

	for _, u := range urls {
		chNumStr, err := ExtractChapterNumber(u)
		if err != nil {
			continue // Skip if no chapter number found
		}
		chNum, err := strconv.Atoi(chNumStr)
		if err != nil {
			continue
		}
		chapters = append(chapters, ChapterInfo{URL: u, ChapterNum: chNum})
	}

	if len(chapters) == 0 {
		return nil, fmt.Errorf("no valid chapters found to sort")
	}

	sort.Slice(chapters, func(i, j int) bool {
		return chapters[i].ChapterNum < chapters[j].ChapterNum
	})

	var filtered []string
	for _, chap := range chapters {
		if (start == 0 || chap.ChapterNum >= start) &&
			(end == 0 || chap.ChapterNum <= end) {
			filtered = append(filtered, chap.URL)
		}
	}

	return filtered, nil
}
