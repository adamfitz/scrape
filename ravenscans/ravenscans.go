package ravenscans

import (
	//"bytes"
	"context"
	//"fmt"
	//"image"
	//"image/jpeg"
	//"io"
	"log"
	//"net/http"
	//"os"
	//"path/filepath"
	"scrape/parser"
	//"strconv"
	//"strings"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	//"github.com/chromedp/cdproto/input"

	//"github.com/chai2010/webp"
	//"github.com/chromedp/cdproto/network"
	"github.com/gocolly/colly"
	_ "golang.org/x/image/webp"
	_ "image/gif" // register GIF decoder
	_ "image/png" // register PNG decoder
)

// returns the chapter urls as a map (filename as the key and url as the value)
func ChapterURLs(mangaUrl string) (map[string]string, error) {
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
func VisitPage(url string) (string, error) {
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
func ExtractChapterImageUrls(pageContent, chapterURL string) []string {
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
