package ravenscans

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"scrape/parser"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/chromedp/chromedp"

	"github.com/gocolly/colly"
	_ "golang.org/x/image/webp"
	_ "image/gif" // register GIF decoder
	_ "image/png" // register PNG decoder
)

// returns the chapter urls as a map (filename as the key and url as the value)
func chapterUrls(mangaUrl string) (map[string]string, error) {
	chapterMap := make(map[string]string)

	c := colly.NewCollector()

	// Debug hooks
	c.OnRequest(func(r *colly.Request) {
		log.Printf("[DEBUG] ravenscans ChapterURLs() - Visiting %s", r.URL.String())
	})
	c.OnResponse(func(r *colly.Response) {
		log.Printf("[DEBUG] ravenscans ChapterURLs() - Response from %s - Status %d", r.Request.URL, r.StatusCode)
	})
	c.OnError(func(r *colly.Response, err error) {
		log.Printf("[ERROR] ravenscans ChapterURLs() - Request to %s failed: %v (Status: %d)", r.Request.URL, err, r.StatusCode)
	})

	c.OnHTML("div.eplister ul li", func(e *colly.HTMLElement) {
		rawNum := e.Attr("data-num")
		url := e.ChildAttr("a", "href")
		title := e.ChildText("div.eph-num span.chapternum")

		filename := parser.CreateFilename(rawNum)

		log.Printf("[INFO] ravenscans ChapterURLs() - Chapter %s -> %s (%s) => filename %s", rawNum, url, title, filename)

		chapterMap[filename] = url
	})

	// visit the chapter page
	err := c.Visit(mangaUrl)
	if err != nil {
		return nil, err
	}

	return chapterMap, nil
}

// Loads a given URL, ensuring all JavaScript and resources are loaded.
func visitPage(url string) (string, error) {
	// Create a new context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Create a timeout context for the operation
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var htmlContent string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		// Wait for the 'networkidle0' event, which signifies that there are no
		// more than 0 network connections for at least 500ms. This helps ensure
		// all resources, including JavaScript, have finished loading.
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.WaitReady("body").Do(ctx) // Wait for the body element to be ready
		}),
		chromedp.OuterHTML("html", &htmlContent), // Get the outer HTML of the entire document
	)
	if err != nil {
		return "", err
	}

	return htmlContent, nil
}

// gret all the chapter image URLs out of the page html, remove any images NOTspecifically for the chapter, deduplicate the
// list and then order them ready for download
func extractChapterImageUrls(pageContent, chapterURL string) []string {
	// Parse the chapter URL
	u, err := url.Parse(chapterURL)
	if err != nil {
		fmt.Println("Invalid chapter URL:", err)
		return nil
	}

	// Extract the last segment as the chapter slug
	chapterSlug := strings.Trim(u.Path, "/")
	chapterSlug = path.Base(chapterSlug)

	// Remove "-chapter-<num>" from the slug to get the base manga slug
	baseSlugRe := regexp.MustCompile(`^(.*)-chapter-\d+$`)
	if matches := baseSlugRe.FindStringSubmatch(chapterSlug); len(matches) == 2 {
		chapterSlug = matches[1]
	}

	// Regex to match only manga page images for this chapter
	imgRe := regexp.MustCompile(fmt.Sprintf(`https://manga\.pics/%s/chapter-[0-9]+/([0-9]+)\.jpg`, regexp.QuoteMeta(chapterSlug)))
	matches := imgRe.FindAllStringSubmatch(pageContent, -1)

	// Map to deduplicate and store page number for sorting
	type imgInfo struct {
		url       string
		pageIndex int
	}
	seen := make(map[string]struct{})
	images := []imgInfo{}

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		url := m[0]
		pageNumStr := m[1]

		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}

		pageNum, err := strconv.Atoi(pageNumStr)
		if err != nil {
			continue
		}

		images = append(images, imgInfo{url: url, pageIndex: pageNum})
	}

	// Sort by page number
	sort.Slice(images, func(i, j int) bool {
		return images[i].pageIndex < images[j].pageIndex
	})

	// Build ordered slice of URLs
	orderedURLs := make([]string, len(images))
	for i, img := range images {
		orderedURLs[i] = img.url
	}

	return orderedURLs
}

func DownloadMangaChapters(mangaUrl string) {
	// Get list of chapters already downloaded (only *.cbz)
	downloadedChapters, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("error getting file list: %v", err)
	}
	downloadedChapters = parser.FilterCBZFiles(downloadedChapters)
	log.Printf("[INFO] raven scans DownloadMangaChapters() - chapters already downloaded: %v, from URL: %s", downloadedChapters, mangaUrl)

	// Get all chapters from website
	chapterMap, err := chapterUrls(mangaUrl)
	if err != nil {
		log.Fatalf("[ERROR] raven scans DownloadMangaChapters() - Get Chapter URLs failed: %v", err)
	}

	// sort the chapters
	sortedChapterList, sortError := parser.SortKeys(chapterMap)
	if sortError != nil {
		log.Fatalf("[ERROR] raven scans DownloadMangaChapters() - failed to srot chapter keys %v", sortError)
	}

	// grab page, extract image urls and download images in the chapter
	//for chapterName, chapterUrl := range chapterMap {
	for _, chapterName := range sortedChapterList {
		chapterUrl := chapterMap[chapterName]
		fmt.Printf("[INFO] raven scans DownloadMangaChapters() - visiting: %s\n", chapterUrl)

		// get the fuillpage content (after JS loading)
		pageContent, _ := visitPage(chapterUrl)

		// extract image urls for the chapter, remove unrealted, deduplicate and sort
		log.Println("[INFO] raven scans DownloadMangaChapters() - extracing, deduplicating and sorting image urls:")
		imageUrls := extractChapterImageUrls(pageContent, chapterUrl)

		// Create temp directory for this chapter's images
		tmpDir, err := os.MkdirTemp("", "chapter-"+chapterName)
		if err != nil {
			log.Printf("[ERROR] raven scans DownloadMangaChapters() - failed to create temp dir for %s: %v", chapterName, err)
			continue
		}

		log.Printf("Starting download for chapter: %s, with %d images", chapterName, len(chapterMap))

		for imgIndex, imageUrl := range imageUrls {
			imgDlErr := downloadAndConvertToJPG(imageUrl, tmpDir, chapterName, imgIndex, len(imageUrls))
			if imgDlErr != nil {
				log.Printf("[ERROR] raven scans DownloadMangaChapters() - error downloading image %d of chapter %s: %v", imgIndex, chapterName, err)
			}
		}
		// Create CBZ from temp dir images
		targetFile := "./" + chapterName
		err = parser.CreateCbzFromDir(tmpDir, targetFile)
		if err != nil {
			log.Printf("[ERROR] raven scans DownloadMangaChapters() - failed to create CBZ for chapter %s: %v", chapterName, err)
		} else {
			log.Printf("[INFO] raven scans DownloadMangaChapters() - Created CBZ file: %s", targetFile)
			fmt.Printf("Created CBZ file: %s\n", targetFile)
		}

		// Clean up temp directory after CBZ creation
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf("[ERROR] raven scans DownloadMangaChapters() - failed to remove temp dir %s: %v", tmpDir, err)
		}
	}
}

func downloadAndConvertToJPG(imageURL, targetDir, chapterName string, imageIndex, totalImages int) error {
	log.Printf("[DOWNLOAD] Chapter %s: Downloading image %d/%d: %s", chapterName, imageIndex, totalImages, imageURL)

	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad response status: %s", resp.Status)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read image data: %w", err)
	}

	format, err := parser.DetectImageFormat(imgBytes)
	if err != nil {
		return fmt.Errorf("failed to detect image format: %w", err)
	}

	log.Printf("[INFO] Detected image format: %s", format)

	paddedIndex := fmt.Sprintf("%03d", imageIndex)
	filename := paddedIndex + ".jpg" // Always save as .jpg to enforce conversion

	outputFile := filepath.Join(targetDir, filename)

	if format == "jpeg" {
		log.Printf("[INFO] Image is already JPEG, saving directly as %s", outputFile)
		err = os.WriteFile(outputFile, imgBytes, 0644)
		if err != nil {
			return fmt.Errorf("failed to save jpeg image: %w", err)
		}
		return nil
	}

	log.Printf("[INFO] Decoding image to convert to JPEG")

	var img image.Image

	switch format {
	case "png", "gif":
		img, _, err = image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode %s image: %w", format, err)
		}
	case "webp":
		img, err = webp.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode webp image: %w", err)
		}
	default:
		return fmt.Errorf("unsupported image format: %s", format)
	}

	log.Printf("[INFO] Encoding image as JPEG and saving to %s", outputFile)

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	opts := jpeg.Options{Quality: 75} // the compression for the jpeg 75 == small file size, 90 == large file size
	err = jpeg.Encode(outFile, img, &opts)
	if err != nil {
		return fmt.Errorf("failed to encode jpeg: %w", err)
	}

	log.Printf("[SUCCESS] Converted and saved image as JPEG: %s", outputFile)

	return nil
}
