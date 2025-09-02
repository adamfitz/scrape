package stonescape

import (
	"context"
	"fmt"
	"log"
	"scrape/parser"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func DownloadChapters(seriesUrl string) {
	// step zero call clean up func
	var tempDirList []string
	parser.CleanupTempDirs(&tempDirList)

	// Step 1: Get chapter map from website
	chapterMap, err := chapterUrls(seriesUrl)
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

		// Fetch chapter images
		chapterImageList, chImgErr := chapterImageUrls(chapterURL)
		if chImgErr != nil {
			log.Fatalf("Failed to get chapter image: %v", chImgErr)
		}

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
		for _, image := range chapterImageList {
			parser.DownloadAndConvertToJPG(image, tempDir)
		}

		// create the cbz file
		parser.CreateCbzFromDir(tempDir, filename)
		fmt.Printf("Downloaded: %s\n", filename)
	}
}

// fetches all chapter URLs for a given StoneScape series URL
// Returns a map of chapter number (string) : chapter URL
func chapterUrls(seriesURL string) (map[string]string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	chapterMap := make(map[string]string)
	var rawChapters []map[string]string

	err := chromedp.Run(ctx,
		chromedp.Navigate(seriesURL),
		chromedp.WaitVisible(`div.listing-chapters_wrap ul.main.version-chap li.wp-manga-chapter a`, chromedp.ByQuery),
		chromedp.Evaluate(`
            [...document.querySelectorAll('div.listing-chapters_wrap ul.main.version-chap li.wp-manga-chapter a')]
            .map(a => {
                const txt = a.textContent.trim();
                if(txt.match(/^Ch\. \d+$/)) {
                    return { num: txt.replace('Ch. ','') , url: a.href };
                }
                return null;
            })
            .filter(x => x !== null)
        `, &rawChapters),
	)
	if err != nil {
		return nil, err
	}

	// Remove duplicates
	seen := make(map[string]struct{})
	for _, chap := range rawChapters {
		num := chap["num"]
		url := chap["url"]
		if _, exists := seen[num]; !exists {
			seen[num] = struct{}{}
			chapterMap[num] = url
		}
	}

	// create the chapter file names and use them as the map keys
	result := chapterMapping(chapterMap)

	return result, nil
}

// Fetches all image URLs from a single StoneScape chapter page
func chapterImageUrls(chapterURL string) ([]string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var imageLinks []string

	err := chromedp.Run(ctx,
		// Navigate to the chapter page
		chromedp.Navigate(chapterURL),

		// Wait for at least one image to be visible
		chromedp.WaitVisible(`img.wp-manga-chapter-img`, chromedp.ByQuery),

		// Extract the src attributes of all images
		chromedp.Evaluate(`
			[...document.querySelectorAll('img.wp-manga-chapter-img')].map(img => img.src.trim())
		`, &imageLinks),
	)
	if err != nil {
		return nil, err
	}

	return imageLinks, nil
}

// take in the chapter URLs map and contructs the chapter file names based on the map key
func chapterMapping(inputMap map[string]string) map[string]string {

	var chapterMap = make(map[string]string)

	for key, value := range inputMap {
		fileName := chapterFileName(key)
		chapterMap[fileName] = value
	}

	return chapterMap
}

// from the chapter number (key in chapterList) return the chapter filename
func chapterFileName(chapterNumber string) string {

	// pad the chapter number
	paddedNum := fmt.Sprintf("%03s", chapterNumber)

	return fmt.Sprintf("ch%s.cbz", paddedNum)

}
