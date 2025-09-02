package cfotz

import (
	"fmt"
	"html"
	"log"
	"regexp"
	"scrape/parser"
	"scrape/webClient"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func DownloadChapters() {
	// step zero call clean up func
	var tempDirList []string
	parser.CleanupTempDirs(&tempDirList)

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

	// Step 4: Loop through each chapter
	for _, filename := range keys {
		if existing[filename] {
			log.Printf("Skipping %s - already exists.", filename)
			continue
		}

		chapterName := strings.Split(filename, ".cbz")

		chapterURL := chapterMap[filename]
		fmt.Printf("Downloading %s\n", chapterName[0])
		log.Printf("Downloading %s from %s", filename, chapterURL)

		// Fetch chapter HTML using webClient.FetchChapterPage
		pageHTML, err := webClient.FetchChapterPage(chapterURL)
		if err != nil {
			log.Printf("Failed to fetch chapter page %s: %v", chapterURL, err)
			continue
		}

		// Parse the chapter page HTML into a new GoQuery document
		chapterPage, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
		if err != nil {
			log.Printf("Failed to parse chapter page HTML: %v", err)
			continue
		}

		// Parse image URLs from chapter page HTML
		// Extract image URLs (specific: figure.wp-block-image)
		var imgURLs []string
		chapterPage.Find("figure.wp-block-image img").Each(func(i int, s *goquery.Selection) {
			src := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
			if src != "" {
				imgURLs = append(imgURLs, src)
				log.Printf("Found image %d: %s", i+1, src)
			}
		})

		// create temp dir name
		dirName := strings.Split(filename, ".cbz")
		tempPrefix := dirName[0]

		// create the temp directory (prefix and random string create teh temp dir name)
		tempDir, err := parser.CreateTempDir(tempPrefix)
		if err != nil {
			log.Printf("could not create temp directory: %s", tempDir)
		}
		// append to the list of temp directories that need to be removed once the download func finishes or
		// crashes/hard exits
		tempDirList = append(tempDirList, tempDir)
		log.Printf("Created tempdir: %s", tempDir)

		// download chapter images to temp directory
		for _, image := range imgURLs {
			parser.DownloadAndConvertToJPG(image, tempDir)
		}

		// create the cbz file
		parser.CreateCbzFromDir(tempDir, filename)
		fmt.Printf("Downloaded: %s\n", filename)
	}
}

// get the chapter URls, using backup func from webclient
func ChapterUrls() (map[string]string, error) {
	baseURL := "https://childhoodfriendofthezenith.org/"

	// Reuse existing retry/backoff function to fetch the HTML
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

	// Compile once outside the loop for performance
	var chapterNumRegex = regexp.MustCompile(`\d+(?:\.\d+)?`)

	// get the chapter URL list, each entries href and chapter name is extracted
	doc.Find("#chapters-list-holder a.chapter-list-item").Each(func(_ int, element *goquery.Selection) {
		href, ok := element.Attr("href")
		if !ok {
			log.Printf("Could not get href attribute %v", element)
			return
		}

		// normalize the chapter text (trim + unescape HTML entities like &#8211;)
		rawName := html.UnescapeString(strings.TrimSpace(element.Find("span.chapter-name").Text()))

		// extract the first number found
		chNum := chapterNumRegex.FindString(rawName)

		// add chapter number and url to map
		result[chNum] = href
	})

	// create the chapter file names and use them as the map keys
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
