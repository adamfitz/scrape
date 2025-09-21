package ravenscans

import (
	//"bytes"
	//"context"
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

	//"github.com/chromedp/chromedp"
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
