// scrape new chapters from Infinite level up in murium website

package iluim

import (
	"archive/zip"
	"context"
	"fmt"
	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"scrape/parser"
	"sort"
	"strconv"
	"strings"
	"time"
)

func extractChapterNumber(href string) string {
	// Extracts chapter numbers from paths like:
	// "/chapter-3", "/chapter-45.5", "/chapter-76-5", etc.

	re := regexp.MustCompile(`chapter[-_/](\d+(?:[.-]\d+)?)`)
	matches := re.FindStringSubmatch(href)
	if len(matches) < 2 {
		return ""
	}

	ch := matches[1]
	ch = strings.ReplaceAll(ch, "-", ".") // Treat hyphen as decimal point

	// Split integer and fractional parts (if any)
	parts := strings.SplitN(ch, ".", 2)
	intPart := parts[0]

	num, err := strconv.Atoi(intPart)
	if err != nil {
		return ""
	}

	padded := fmt.Sprintf("%03d", num)

	if len(parts) == 2 {
		// Reattach fractional part
		return "ch" + padded + "." + parts[1]
	}
	return "ch" + padded
}

func DownloadChapters(chapterURLs []string) error {
	chapterMap := make(map[string]string)

	fmt.Println("Retrieving chapter list...")
	for _, url := range chapterURLs {
		chapterNum := extractChapterNumber(url)
		if chapterNum == "" {
			log.Printf("Warning: could not extract chapter number from URL: %s", url)
			fmt.Printf("Warning: could not extract chapter number from URL: %s", url)
			continue
		}
		chapterMap[chapterNum] = url
	}

	// get a list of the chapter in the current directory
	downloadedChapters, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("error getting file list: %v", err)
	}

	for _, chName := range downloadedChapters {
		// trim ext instead of split so it does not remove the in between chapters eg .5 .6 etc
		baseName := strings.TrimSuffix(chName, ".cbz")

		// remove chapter number from the chapter map if it is already downloaded
		if _, ok := chapterMap[baseName]; ok {
			delete(chapterMap, baseName)
			log.Printf("%s already downloaded, removed from chapterMap", baseName)
		}
	}

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(chapterMap))
	for k := range chapterMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, chapterNum := range keys {
		chapterURL := chapterMap[chapterNum]

		fmt.Println("Starting download for chapter: ", chapterNum)

		tmpDir, err := os.MkdirTemp("", "chapter-"+chapterNum)
		if err != nil {
			log.Printf("Failed to create temp dir for %s: %v", chapterNum, err)
			fmt.Printf("Failed to create temp dir for %s: %v", chapterNum, err)
			continue
		}
		defer os.RemoveAll(tmpDir)

		ctx, cancel := chromedp.NewContext(context.Background())
		defer cancel()

		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		var html string
		if err := chromedp.Run(ctx,
			chromedp.Navigate(chapterURL),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second/2), // sleep 500ms?
			chromedp.OuterHTML("html", &html),
		); err != nil {
			log.Printf("chromedp navigation failed for %s: %v", chapterURL, err)
			fmt.Printf("chromedp navigation failed for %s: %v", chapterURL, err)
			continue
		}

		re := regexp.MustCompile(`data-src=["'](https?://[^"']+\.(?:jpg|jpeg|png|webp))["']`)
		matches := re.FindAllStringSubmatch(html, -1)
		if len(matches) == 0 {
			log.Printf("no images found for chapter %s at %s", chapterNum, chapterURL)
			fmt.Printf("no images found for chapter %s at %s", chapterNum, chapterURL)
			continue
		}

		fmt.Printf("Starting download of chapter %s images...\n", chapterNum)
		var imgPaths []string
		for i, match := range matches {
			imgURL := match[1]
			lowerURL := strings.ToLower(imgURL)

			// Skip common icon or social media domains or filenames
			skip := false
			skipPatterns := []string{
				"facebook", "twitter", "linkedin", "pinterest",
				"icon", "favicon", "logo", "sprite", "social", "avatar",
			}
			for _, pattern := range skipPatterns {
				if strings.Contains(lowerURL, pattern) {
					log.Printf("Skipping unwanted image: %s", imgURL)
					skip = true
					break
				}
			}
			if skip {
				continue
			}

			// remove erroneous chars  like \n
			cleanedURL := strings.ReplaceAll(imgURL, "\n", "")
			cleanedURL = strings.TrimSpace(cleanedURL)
			// validate URL before trying to use it
			_, err := url.ParseRequestURI(cleanedURL)
			if err != nil {
				log.Printf("Invalid URL: %s → %v", cleanedURL, err)
				return err
			}

			resp, err := http.Get(cleanedURL)
			if err != nil {
				log.Printf("Failed to download image %s: %v", cleanedURL, err)
				fmt.Printf("Failed to download image %s: %v", cleanedURL, err)
				continue
			}

			img, _, err := image.Decode(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("Failed to decode image %s: %v", cleanedURL, err)
				fmt.Printf("Failed to decode image %s: %v", cleanedURL, err)
				continue
			}

			filename := fmt.Sprintf("img%03d.jpg", i+1)
			fullPath := filepath.Join(tmpDir, filename)

			out, err := os.Create(fullPath)
			if err != nil {
				log.Printf("Failed to create file %s: %v", fullPath, err)
				fmt.Printf("Failed to create file %s: %v", fullPath, err)
				continue
			}
			if err := jpeg.Encode(out, img, &jpeg.Options{Quality: 90}); err != nil {
				log.Printf("Failed to encode image %s: %v", fullPath, err)
				fmt.Printf("Failed to encode image %s: %v", fullPath, err)
			}
			out.Close()
			imgPaths = append(imgPaths, fullPath)
		}

		if len(imgPaths) == 0 {
			log.Printf("no images could be saved for chapter %s at %s", chapterNum, chapterURL)
			fmt.Printf("no images could be saved for chapter %s at %s", chapterNum, chapterURL)
			continue
		}

		cbzName := fmt.Sprintf("%s.cbz", chapterNum)
		if err := createCbz(imgPaths, cbzName); err != nil {
			log.Printf("failed to create cbz for chapter %s: %v", chapterNum, err)
			continue
		}

		log.Printf("Chapter %s downloaded and saved as %s", chapterNum, cbzName)
		fmt.Printf("Chapter %s downloaded and saved as %s\n", chapterNum, cbzName)
	}

	return nil
}

func createCbz(imgPaths []string, zipName string) error {
	zipFile, err := os.Create(zipName)
	if err != nil {
		return fmt.Errorf("failed to create cbz file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, img := range imgPaths {
		err := func() error {
			f, err := os.Open(img)
			if err != nil {
				return err
			}
			defer f.Close()

			w, err := zipWriter.Create(filepath.Base(img))
			if err != nil {
				return err
			}

			_, err = io.Copy(w, f)
			return err
		}()
		if err != nil {
			return fmt.Errorf("error adding %s to cbz: %w", img, err)
		}
	}

	return nil
}

// Get the chatper URLs return string slice
func ChapterURLs(mangaURL string) ([]string, error) {
	var chapterURLs []string

	c := colly.NewCollector()

	// Target the <a> tags inside the second-level <ul> within the 'ceo_latest_comics_widget' widget
	// which is itself inside the div with id 'Chapters_List'
	c.OnHTML("div#Chapters_List > ul > li.widget.ceo_latest_comics_widget > ul > li > a", func(e *colly.HTMLElement) {
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
