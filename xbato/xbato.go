// module for scraping xbato.com
package xbato

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"scrape/parser"
	"slices"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly"
)

// get list of all the chapter URLss
func XbatoChapterUrls(mangaName string) ([]string, error) {
	var urls []string

	mangaURL := fmt.Sprintf("https://xbato.com/series/%s", mangaName)

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"),
	)

	c.OnHTML("div.episode-list div.main a.visited.chapt", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		if href != "" {
			fullURL := "https://xbato.com" + href
			urls = append(urls, fullURL)
		}
	})

	var scrapeErr error
	c.OnError(func(_ *colly.Response, err error) {
		scrapeErr = err
	})

	err := c.Visit(mangaURL)
	if err != nil {
		return nil, err
	}

	if scrapeErr != nil {
		return nil, scrapeErr
	}

	return urls, nil
}

// The list of chapters to the corresponding chapter number in the URL is contained in the chapters options HTML on the
// chapter page itself
func ChapterOptions(chapterURL string) (map[string]string, error) {
	chapters := make(map[string]string)

	c := colly.NewCollector()

	c.OnHTML("optgroup[label='Chapters'] option", func(e *colly.HTMLElement) {
		value := e.Attr("value")
		text := e.Text
		if value != "" {
			chapters[value] = text
		}
	})

	var scrapeErr error
	c.OnError(func(_ *colly.Response, err error) {
		scrapeErr = err
	})

	err := c.Visit(chapterURL)
	if err != nil {
		return nil, err
	}

	if scrapeErr != nil {
		return nil, scrapeErr
	}

	return chapters, nil
}

// Formats the resulting resulting chapter map to name the chapters in uniform format eg: ch01, ch02, ch20 etc
func FormatChapterMap(chapters map[string]string) map[string]string {
	formatted := make(map[string]string)

	// Matches volume and chapter numbers, including decimals
	volChapRegex := regexp.MustCompile(`(?i)volume\s*(\d+)[^\d]*chapter\s*(\d+(?:\.\d+)?)`)
	chapOnlyRegex := regexp.MustCompile(`(?i)chapter\s*(\d+(?:\.\d+)?)`)

	for id, text := range chapters {
		normalized := strings.ToLower(text)

		if matches := volChapRegex.FindStringSubmatch(normalized); len(matches) == 3 {
			vol := matches[1]
			ch := matches[2]

			// Pad volume
			if len(vol) == 1 {
				vol = "0" + vol
			}

			// Pad chapter left of dot (e.g., 3.5 → 03.5)
			if parts := strings.Split(ch, "."); len(parts) > 1 && len(parts[0]) == 1 {
				ch = "0" + ch
			} else if len(ch) == 1 {
				ch = "0" + ch
			}

			formatted[id] = fmt.Sprintf("vol%sch%s", vol, ch)
			continue
		}

		if matches := chapOnlyRegex.FindStringSubmatch(normalized); len(matches) == 2 {
			ch := matches[1]

			if parts := strings.Split(ch, "."); len(parts) > 1 && len(parts[0]) == 1 {
				ch = "0" + ch
			} else if len(ch) == 1 {
				ch = "0" + ch
			}

			formatted[id] = fmt.Sprintf("ch%s", ch)
			continue
		}

		// fallback: use raw text
		formatted[id] = strings.ReplaceAll(normalized, " ", "_")
	}

	return formatted
}

// Creates a CBZ file
func createCbz(sourceDir, zipName string) error {
	// Ensure zipName ends with .cbz
	if filepath.Ext(zipName) != ".cbz" {
		zipName = zipName + ".cbz"
	}

	zipFile, err := os.Create(zipName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		f, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		fsFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fsFile.Close()

		_, err = io.Copy(f, fsFile)
		return err
	})
}

// Extract chapter ID from URL
// Example URL: "https://xbato.com/chapter/2013166/some-chapter-title"
// We want to extract "2013166"
func extractChapterID(chapterURL string) string {
	u, err := url.Parse(chapterURL)
	if err != nil {
		return ""
	}
	// Example path: "/chapter/2013166/some-chapter-title"
	parts := strings.Split(u.Path, "/")
	for i, p := range parts {
		if p == "chapter" && i+1 < len(parts) {
			return parts[i+1] // returns the numeric ID after "chapter"
		}
	}
	return ""
}

// Download a file (chapter image)
func downloadFile(url string, filePath string) error {
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	c := colly.NewCollector()
	c.OnResponse(func(r *colly.Response) {
		_, err = out.Write(r.Body)
	})
	return c.Visit(url)
}

func DownloadAndCreateCBZ(chapterURLs []string, chapterMap map[string]string) error {

	// Get list of chapters already downloaded (only *.cbz)
	downloadedChapters, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("error getting file list: %v", err)
	}

	// filter out non cbz files
	filteredCbzFiles := parser.FilterCBZFiles(downloadedChapters)

	for _, url := range chapterURLs {
		chapterName := chapterMap[extractChapterID(url)]

		// dont try and download chapter if it already exists
		if slices.Contains(filteredCbzFiles, chapterName+".cbz") {
			continue
		}

		fmt.Println(chapterName)
		fmt.Printf("Downloading chapter: %s\n", chapterName)
		if chapterName == "" {
			chapterName = "chapter"
		}

		tempDir := filepath.Join(os.TempDir(), chapterName)
		os.MkdirAll(tempDir, os.ModePerm)

		// Use your GetChapterImageUrls func to get image URLs from the page
		imgLinks, err := GetChapterImageUrls(url)
		if err != nil {
			return fmt.Errorf("failed to get image URLs for %s: %w", url, err)
		}

		// Download images
		for i, link := range imgLinks {
			fileName := fmt.Sprintf("%03d.jpg", i+1)
			filePath := filepath.Join(tempDir, fileName)
			err := downloadFile(link, filePath)
			if err != nil {
				log.Printf("Failed downloading image: %v", err)
				continue
			}
		}

		// Create CBZ archive
		cbzName := fmt.Sprintf("%s.cbz", chapterName)
		err = createCbz(tempDir, cbzName)
		if err != nil {
			return err
		}

		// Cleanup temp images
		os.RemoveAll(tempDir)

		fmt.Printf("Created CBZ: %s\n", cbzName)
	}
	return nil
}

// Use chromedp (headless browser) to worka round the java script BS to get the images from the page)
func GetChapterImageUrls(url string) ([]string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var attrsList []map[string]string
	sel := `img.page-img`

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(sel, chromedp.ByQuery),
		chromedp.AttributesAll(sel, &attrsList, chromedp.ByQueryAll, chromedp.AtLeast(1)),
	)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, attrs := range attrsList {
		if val, ok := attrs["src"]; ok {
			urls = append(urls, val)
		}
	}

	return urls, nil
}
