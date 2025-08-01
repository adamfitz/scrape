package manhuaus

import (
	"archive/zip"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	tmpDir, err := os.MkdirTemp("", "manga_chapter")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	c := colly.NewCollector()

	var imgURLs []string
	var chapterValue string

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

	ticker := time.NewTicker(1500 * time.Millisecond)
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

		img, err := DecodeImage(bodyBytes, url)
		if err != nil {
			return err
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

	cbzFileName := fmt.Sprintf("ch%s.cbz", strings.TrimPrefix(chapterValue, "chapter-"))
	if err := CreateCbzFile(tmpDir, cbzFileName); err != nil {
		return err
	}

	fmt.Println("CBZ file created:", cbzFileName)
	return nil
}

// DecodeImage detects the format (JPEG, PNG, WebP) and returns a decoded image.
func DecodeImage(data []byte, sourceURL string) (image.Image, error) {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return jpeg.Decode(bytes.NewReader(data))
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}) {
		return png.Decode(bytes.NewReader(data))
	}
	return webp.Decode(bytes.NewReader(data))
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
