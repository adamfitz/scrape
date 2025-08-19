package manhuaus

import (
	"archive/zip"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bytes"
	"github.com/PuerkitoBio/goquery"
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
// implements Fetch with backup utils code, to rery when hitting dealine exceeded issues
func DownloadChaper(chapterURL, cbzFileName string) error {
	tmpDir, err := os.MkdirTemp("", "manga_chapter")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Fetch chapter page HTML with retry/backoff
	pageHTML, err := webClient.FetchChapterPage(chapterURL)
	if err != nil {
		log.Printf("❌ Error fetching chapter page %s: %v", chapterURL, err)
		return err
	}

	// Parse HTML using goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return fmt.Errorf("failed to parse chapter HTML: %v", err)
	}

	var imgURLs []string
	//var chapterValue string

	chapterValue := strings.TrimSpace(doc.Find("input#wp-manga-current-chap").AttrOr("value", ""))
	if chapterValue == "" {
		return fmt.Errorf("chapter value not found")
	}

	doc.Find("div.reading-content img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("data-src", ""))
		if src != "" {
			imgURLs = append(imgURLs, src)
		}
	})

	if len(imgURLs) == 0 {
		return fmt.Errorf("no images found")
	}

	log.Printf("Found %d images in chapter %s", len(imgURLs), chapterValue)

	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{Timeout: 30 * time.Second}

	for i, url := range imgURLs {
		<-ticker.C

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Printf("⚠️ Failed to create request for %s: %v", url, err)
			continue
		}

		bodyBytes, err := webClient.FetchWithBackoff(client, req)
		if err != nil {
			log.Printf("⚠️ Chapter %s: failed to fetch image %s: %v", chapterValue, url, err)
			continue
		}

		img, err := DecodeImage(bodyBytes, url)
		if err != nil || img == nil {
			log.Printf("⚠️ Chapter %s: failed to decode image %s: %v", chapterValue, url, err)
			continue
		}

		fileName := fmt.Sprintf("page-%03d.jpg", i+1)
		filePath := filepath.Join(tmpDir, fileName)

		outFile, err := os.Create(filePath)
		if err != nil {
			log.Printf("⚠️ Failed to create file %s: %v", filePath, err)
			continue
		}

		err = jpeg.Encode(outFile, img, &jpeg.Options{Quality: 90})
		outFile.Close()
		if err != nil {
			log.Printf("⚠️ Failed to save image %s: %v", url, err)
			continue
		}

		log.Printf("Saved: %s", fileName)
	}

	log.Printf("Writing CBZ to: %s", cbzFileName)
	if err := CreateCbzFile(tmpDir, cbzFileName); err != nil {
		return fmt.Errorf("failed to create CBZ: %v", err)
	}

	log.Printf("CBZ file created: %s", cbzFileName)
	return nil
}

// DecodeImage detects the format (JPEG, PNG, WebP) and returns a decoded image.
func DecodeImage(data []byte, sourceURL string) (image.Image, error) {
	if len(data) < 12 {
		fmt.Printf("⚠️ Skipping image %s — too small (%d bytes)\n", sourceURL, len(data))
		return nil, nil
	}

	// Detect HTML masquerading as image
	if bytes.Contains(data[:min(512, len(data))], []byte("<html")) {
		fmt.Printf("⚠️ Skipping image %s — looks like HTML content, not an image\n", sourceURL)
		return nil, nil
	}

	header := data[:min(16, len(data))]

	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		fmt.Println("Detected image format: JPEG")
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			fmt.Printf("❌ Failed to decode JPEG from %s: %v\n", sourceURL, err)
			return nil, nil
		}
		return img, nil

	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		fmt.Println("Detected image format: PNG")
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			fmt.Printf("❌ Failed to decode PNG from %s: %v\n", sourceURL, err)
			return nil, nil
		}
		return img, nil

	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		fmt.Println("Detected image format: WebP")
		img, err := webp.Decode(bytes.NewReader(data))
		if err != nil {
			fmt.Printf("❌ Failed to decode WebP from %s: %v\n", sourceURL, err)
			return nil, nil
		}
		return img, nil

	default:
		fmt.Printf("❌ Unknown image format from %s\nHeader: %x\n", sourceURL, header)
		return nil, nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CreateCbzFile zips all the files from the tmpDir into cbzFileName.
func CreateCbzFile(tmpDir, cbzFileName string) error {
	cbzFile, err := os.Create(cbzFileName)
	if err != nil {
		return err
	}
	defer cbzFile.Close()

	zipWriter := zip.NewWriter(cbzFile)
	defer zipWriter.Close()

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

// ExtractChapterNumber extracts and formats the chapter number from the URL.
func ExtractChapterNumber(url string) (string, error) {
	re := regexp.MustCompile(`chapter-([\d.]+)`)
	match := re.FindStringSubmatch(url)
	if len(match) < 2 {
		return "", fmt.Errorf("chapter number not found in URL")
	}

	raw := match[1]
	if strings.Contains(raw, ".") {
		parts := strings.SplitN(raw, ".", 2)
		intPart, err := strconv.Atoi(parts[0])
		if err != nil {
			return "", fmt.Errorf("invalid integer part in chapter number: %v", err)
		}
		return fmt.Sprintf("ch%03d.%s.cbz", intPart, parts[1]), nil
	}

	num, err := strconv.Atoi(raw)
	if err != nil {
		return "", fmt.Errorf("invalid chapter number: %v", err)
	}

	return fmt.Sprintf("ch%03d.cbz", num), nil
}
