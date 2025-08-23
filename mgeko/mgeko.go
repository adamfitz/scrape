package mgeko

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"scrape/parser"
	"strings"

	"github.com/gocolly/colly"
)

func DownloadChapters(url string) {
	// Step 1: Get all chapter URLs
	chapterUrls, err := chapterUrls(url)
	if err != nil {
		log.Fatalf("Failed to fetch chapter URLs: %v", err)
	}

	// Step 2: Build chapter map (key = "chXXX.cbz", value = URL)
	chapterMap := chapterMap(chapterUrls)

	// Step 3: Get list of files in current dir
	currentFiles, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("Failed to list files: %v", err)
	}

	// Step 4: Filter all non-CBZ files
	filteredCbzFiles := parser.FilterCBZFiles(currentFiles)

	// Step 5: Remove already-downloaded chapters
	for _, chapter := range filteredCbzFiles {
		delete(chapterMap, chapter)
	}

	// Step 6: Sort chapter keys
	sortedChapters, sortError := parser.SortKeys(chapterMap)
	if sortError != nil {
		log.Fatalf("Failed to sort chapter map keys: %v", sortError)
	}

	// Step 7: Iterate over sorted chapter keys
	for _, cbzName := range sortedChapters {
		chapterURL := chapterMap[cbzName]
		fmt.Printf("Downloading chapter %s -> %s\n", cbzName, chapterURL)

		// Colly to scrape image URLs inside #chapter-reader
		var imgURLs []string
		c := colly.NewCollector(
			colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"),
		)
		c.OnHTML("#chapter-reader img", func(e *colly.HTMLElement) {
			src := e.Attr("src")
			if src != "" {
				imgURLs = append(imgURLs, src)
				log.Printf("[%s] Found image URL: %s", cbzName, src)
			}
		})
		c.OnError(func(_ *colly.Response, err error) {
			log.Printf("[%s] Failed to fetch chapter page %s: %v", cbzName, chapterURL, err)
		})

		err := c.Visit(chapterURL)
		if err != nil {
			log.Printf("[%s] Failed to visit %s: %v", cbzName, chapterURL, err)
			continue
		}

		if len(imgURLs) == 0 {
			log.Printf("[%s] No images found for chapter", cbzName)
			continue
		}

		// Create temp directory for chapter
		chapterDir := filepath.Join("/tmp", strings.TrimSuffix(cbzName, ".cbz"))
		err = os.MkdirAll(chapterDir, 0755)
		if err != nil {
			log.Printf("[%s] Failed to create temporary directory %s: %v", cbzName, chapterDir, err)
			continue
		}

		// Download and convert each image using DownloadAndConvertToJPG
		for idx, imgURL := range imgURLs {
			log.Printf("[%s] Downloading image %d/%d: %s", cbzName, idx+1, len(imgURLs), imgURL)
			err := parser.DownloadAndConvertToJPG(imgURL, chapterDir)
			if err != nil {
				log.Printf("[%s] Failed to download/convert image %s: %v", cbzName, imgURL, err)
			} else {
				log.Printf("[%s] Successfully downloaded and converted image: %s", cbzName, imgURL)
			}
		}

		// Create CBZ from the JPGs
		err = parser.CreateCbzFromDir(chapterDir, cbzName)
		if err != nil {
			log.Printf("[%s] Failed to create CBZ %s: %v", cbzName, cbzName, err)
		} else {
			fmt.Printf("Created CBZ: %s\n", cbzName)
		}

		// Remove temp directory
		err = os.RemoveAll(chapterDir)
		if err != nil {
			log.Printf("[%s] Failed to remove temp directory %s: %v", cbzName, chapterDir, err)
		}
	}
}

// retrieve mgeko chapter list
func chapterUrls(url string) ([]string, error) {
	var chapters []string

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"),
	)

	c.OnHTML("ul.chapter-list li a", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		if href != "" {
			fullURL := "https://www.mgeko.cc" + href
			chapters = append(chapters, fullURL)
		}
	})

	var scrapeErr error
	c.OnError(func(_ *colly.Response, err error) {
		scrapeErr = err
	})

	err := c.Visit(url)
	if err != nil {
		return nil, err
	}

	if scrapeErr != nil {
		return nil, scrapeErr
	}

	return chapters, nil
}

// chapterMap takes a slice of Mgeko reader URLs and returns a map:
// key = "ch<number>.cbz" (with integer part padded to 3 digits), value = URL
func chapterMap(urls []string) map[string]string {
	chapterMap := make(map[string]string)
	re := regexp.MustCompile(`chapter-([0-9]+(?:\.[0-9]+)?)`)

	for _, url := range urls {
		matches := re.FindStringSubmatch(url)
		if len(matches) > 1 {
			numStr := matches[1]
			parts := strings.Split(numStr, ".")
			intPart := parts[0]
			decPart := ""
			if len(parts) > 1 {
				decPart = "." + parts[1] // keep decimal if exists
			}

			// Pad integer part to 3 digits
			filename := fmt.Sprintf("ch%03s%s.cbz", intPart, decPart)
			chapterMap[filename] = url
		}
	}

	return chapterMap
}

// parses chapter HTML and returns slice of image URLs
func extractChapterImageUrls(html string) []string {
	var urls []string
	re := regexp.MustCompile(`<img\s+[^>]*id="image-[0-9]+"[^>]*src="([^"]+)"`)
	matches := re.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) > 1 {
			urls = append(urls, m[1])
		}
	}
	return urls
}
