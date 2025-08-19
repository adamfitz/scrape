package hls

import (
	"fmt"
	"image/png"
	"log"
	"os"
	"strings"
	"time"

	"net/http"
	"path/filepath"
	"scrape/parser"
	"scrape/webClient"

	"github.com/PuerkitoBio/goquery"
)

func DownloadChapters() {
	// Step 1: Get chapter map from website
	chapterMap, err := ChapterUrls()
	if err != nil {
		log.Fatalf("Failed to download chapter list: %v", err)
	}

	// Step 2: Sort keys descending
	keys, err := parser.SortKeys(chapterMap)
	if err != nil {
		log.Fatalf("Failed to sort chapters: %v", err)
	}

	// Step 3: Get existing CBZ files to skip already downloaded
	existing, err := parser.GetDownloadedCBZ(".")
	if err != nil {
		log.Fatalf("Failed to read current directory: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Step 4: Loop through each chapter
	for _, filename := range keys {
		if existing[filename] {
			log.Printf("Skipping %s - already exists.", filename)
			continue
		}

		chapterURL := chapterMap[filename]
		log.Printf("Downloading %s from %s", filename, chapterURL)

		// Fetch chapter HTML using webClient.FetchChapterPage
		pageHTML, err := webClient.FetchChapterPage(chapterURL)
		if err != nil {
			log.Printf("Failed to fetch chapter page %s: %v", chapterURL, err)
			continue
		}

		snippet := pageHTML
		if len(pageHTML) > 512 {
			snippet = pageHTML[:512] // log only first 512 chars
		}
		log.Printf("HTML snippet for %s:\n%s", filename, snippet)

		// Parse HTML
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
		if err != nil {
			log.Printf("Failed to parse chapter HTML %s: %v", chapterURL, err)
			continue
		}

		// Extract image URLs (robust: div#content and div.reading-content)
		var imgURLs []string
		doc.Find("div#content img, div.reading-content img").Each(func(i int, s *goquery.Selection) {
			src := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
			if src != "" {
				imgURLs = append(imgURLs, src)
				log.Printf("Found image %d: %s", i+1, src)
			}
		})

		if len(imgURLs) == 0 {
			log.Printf("No images found for %s", filename)
			continue
		}

		// Create temporary directory for this chapter
		tmpDir, err := os.MkdirTemp("", "manga_chapter")
		if err != nil {
			log.Printf("Failed to create temp dir for %s: %v", filename, err)
			continue
		}
		// Download each image using webClient.FetchWithBackoff
		fmt.Println("Downloading chapter images...")
		for i, url := range imgURLs {
			req, _ := http.NewRequest("GET", url, nil)

			bodyBytes, err := webClient.FetchWithBackoff(client, req)
			if err != nil {
				log.Printf("Failed to fetch image %s: %v", url, err)
				continue
			}

			img, err := parser.DecodeImageToPng(bodyBytes, url)
			if err != nil || img == nil {
				log.Printf("Failed to decode image %s: %v", url, err)
				continue
			}

			fileName := fmt.Sprintf("page-%03d.png", i+1)
			filePath := filepath.Join(tmpDir, fileName)

			outFile, err := os.Create(filePath)
			if err != nil {
				log.Printf("Failed to create file %s: %v", filePath, err)
				continue
			}

			if err := png.Encode(outFile, img); err != nil {
				log.Printf("Failed to save image %s: %v", url, err)
			}
			outFile.Close()
			log.Printf("Saved image %s", fileName)
		}

		// Create CBZ
		if err := parser.CreateCbzFromDir(tmpDir, filename); err != nil {
			log.Printf("Failed to create CBZ %s: %v", filename, err)
		} else {
			fmt.Printf("Downloaded: %s\n", filename)
		}

		// Delete temp directory
		os.RemoveAll(tmpDir)
	}
}

// get teh chapter URls, using backup func from webclient
func ChapterUrls() (map[string]string, error) {
	baseURL := "https://honeylemonsoda.xyz/"

	// Reuse your existing retry/backoff function to fetch the HTML
	pageHTML, err := webClient.FetchChapterPage(baseURL)
	if err != nil {
		return nil, err
	}

	// Parse HTML with goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return nil, fmt.Errorf("goquery parse error: %w", err)
	}

	result := make(map[string]string)

	doc.Find("li.item a").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		// chop trailing slash and split on "-": last part is the chapter number
		parts := strings.Split(strings.TrimRight(href, "/"), "-")
		if len(parts) == 0 {
			return
		}
		chNum := parts[len(parts)-1]
		result[chNum] = href
	})

	// create the vhapter file names and use them as teh map keys
	chapterMap := chapterMap(result)

	return chapterMap, nil
}

// take in the chapter URls map and contructs the chapter file names based on the map key
func chapterMap(inputMap map[string]string) map[string]string {

	var chapterMap = make(map[string]string)

	for key, value := range inputMap {
		fileName := ChapterFileName(key)
		chapterMap[fileName] = value
	}

	return chapterMap
}

// from the chapter number (key in chapterList) return the chapter filename
func ChapterFileName(chapterNumber string) string {

	// pad the chapter number
	paddedNum := fmt.Sprintf("%03s", chapterNumber)

	return fmt.Sprintf("ch%s.cbz", paddedNum)

}
