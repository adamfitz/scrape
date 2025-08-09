package orv

import (
	"context"
	"fmt"
	"github.com/gocolly/colly"
	"log"
	"os"
	"scrape/parser"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Get the chatper URLs return string slice
func chapterURLs() (map[string]string, error) {
	// resulting chapter map
	var chapterMap = make(map[string]string)
	var chName = ""

	mangaUrl := "https://manhwa.omniscientsreadersmanga.com/"

	c := colly.NewCollector()

	// Debug hooks
	c.OnRequest(func(r *colly.Request) {
		log.Printf("[DEBUG] Visiting %s", r.URL.String())
	})
	c.OnResponse(func(r *colly.Response) {
		log.Printf("[DEBUG] Response from %s - Status %d", r.Request.URL, r.StatusCode)
	})
	c.OnError(func(r *colly.Response, err error) {
		log.Printf("[ERROR] Request to %s failed: %v (Status: %d)", r.Request.URL, err, r.StatusCode)
	})

	// go to teh list-body div, the scroll-sm unordered list and then to the list item
	c.OnHTML("div.list-body ul.scroll-sm li.item", func(e *colly.HTMLElement) {

		// extract the chapter number from the list item
		chapterNum := e.Attr("data-number")

		// extract the string URL from the A HREF child attribute of the list item
		url := e.ChildAttr("a", "href")

		// safely type cast the chapter string to an integer:
		num, err := strconv.Atoi(chapterNum)
		if err != nil {
			log.Printf("orv.ChapterURLs() - error converting chapter string to integer ")
			return
		}

		//  if the chapter number exists then pad it to 3 digits and create the file name
		if chapterNum != "" {
			padded := fmt.Sprintf("%03d", num)
			chName = padded + "." + "cbz"
		}

		// append the chapter number and URL assuming they are not null
		if url != "" && chapterNum != "" {

			chapterMap[chName] = url
		}
	})

	err := c.Visit(mangaUrl)
	if err != nil {
		return nil, err
	}

	if len(chapterMap) == 0 {
		return nil, fmt.Errorf("no chapter URLs found at %s", mangaUrl)
	}

	return chapterMap, nil
}

// return all the image URLs for the chapter
func chapterImageUrls(chapterUrl string) ([]string, error) {
	// Create context with cancel
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Enable network domain to listen to requests/responses (optional)
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		return nil, err
	}

	// Setup listeners for network requests & responses to log them (optional)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			log.Printf("REQUEST: %s %s\n", ev.Request.Method, ev.Request.URL)
		case *network.EventResponseReceived:
			log.Printf("RESPONSE: %d %s\n", ev.Response.Status, ev.Response.URL)
		}
	})

	var imageURLs []string

	// Run chromedp tasks to navigate and extract image URLs
	err := chromedp.Run(ctx,
		chromedp.Navigate(chapterUrl),
		chromedp.WaitReady("img", chromedp.ByQueryAll),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('img')).map(img => img.src)`, &imageURLs),
	)
	if err != nil {
		return nil, err
	}

	return imageURLs, nil
}

// main func to downlownd the orv manga chapters
func DownloadMangaChapters() {

	// get a list of the chapter in the current directory
	downloadedChapters, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("error getting file list: %v", err)
	}

	// get a list of all the chapters from the website
	chapterMap, err := chapterURLs()
	if err != nil {
		log.Fatalf("Get Chapter URls failed: %v", err)
	}

	// remove all chapters that have already been downloaded from the chapter map
	for _, chName := range downloadedChapters {
		// remove chapter number from the chapter map if it is already downloaded
		if _, ok := chapterMap[chName]; ok {
			delete(chapterMap, chName)
			log.Printf("%s already downloaded, removed from chapterMap", chName)
		}
	}

	// after removing any chapters that have already been downloaded
	// sort chapter keys to parse oldest to newest
	chapterNames, err := parser.SortKeys(chapterMap)
	if err != nil {
		log.Fatal("error when sorting chapter name slice")
	}

	// download any new chapters
	for _, chapter := range chapterNames {
		chapterImageUrls, _ := chapterImageUrls(chapterMap[chapter])

		// create chapter number to present to user
		chapterNum := strings.SplitN(chapter, ".", 2)
		fmt.Println("Starting download for chapter: ", chapterNum[0])

		// create temp dir for image file download
		tmpDir, err := os.MkdirTemp("", "chapter-"+chapterNum[0])
		if err != nil {
			log.Printf("Failed to create temp dir for %s: %v", chapterNum, err)
			fmt.Printf("Failed to create temp dir for %s: %v", chapterNum, err)
			continue
		}
		defer os.RemoveAll(tmpDir)

		// this part is teh image download so only download teh images here
		for _, url := range chapterImageUrls {

			// download all the chapter images to temp dir
			parser.DownloadAndConvertToJPG(url, tmpDir)
		}
		// after all the images in the chapter are downloaded
		// create cbz and move to the target dir (the current dir)
		targetFile := "./" + chapter
		parser.CreateCbzFromDir(tmpDir, targetFile)

		fmt.Printf("%s chapter file created...\n", targetFile)
	}
}
