// pick me up infinite gatcha - https://w2.pickmeupgacha.com/

package pmuig

import (
	"fmt"
	//"html"
	"log"
	"net/http"
	//"regexp"
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adamfitz/scrape/parser"
	"github.com/adamfitz/scrape/webClient"

	"github.com/PuerkitoBio/goquery"
	"github.com/chai2010/webp"
	// register WEBP decoder, add module
)

func chapterUrls() (map[string]string, error) {
	baseURL := "https://w2.pickmeupgacha.com/"

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
	//var chapterNumRegex = regexp.MustCompile(`\d+(?:\.\d+)?`)

	// get the chapter URL list, each entry href and chapter name is extracted
	doc.Find("div.elementor-posts-container article.elementor-post h3.elementor-post__title a").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		chapterName := strings.TrimSpace(s.Text())

		result[chapterName] = href
	})

	// create the chapter file names and use them as the map keys
	chapterMap := chapterMap(result)

	return chapterMap, nil
}

// take in the chapter URls map and contructs the chapter file names based on the map key
func chapterMap(inputMap map[string]string) map[string]string {

	var chapterMap = make(map[string]string)
	var number = ""

	for key, value := range inputMap {
		// split the chapter name link (from the AHREF text) string on whitespace characters (the chapter number is last)
		chapterNum := strings.Fields(key)
		if len(chapterNum) > 0 {
			number = chapterNum[len(chapterNum)-1]
		} else {
			log.Printf("Chapter number input was empty or contained only whitespace")
		}

		fileName := chapterFileName(number)
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

// chapterImages loads a chapter page and extracts all image URLs in reading order.
// Returns a map[int]string where the key is the image number and the value is the image URL.
func chapterImages(chapterURL string) (map[int]string, error) {
	resp, err := http.Get(chapterURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chapter page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad HTTP status: %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chapter HTML: %w", err)
	}

	imageMap := make(map[int]string)
	index := 1

	// Each real manga image is in an <img class="lazy"> with data-src
	doc.Find("div.elementor-widget-container img.lazy").Each(func(_ int, s *goquery.Selection) {
		imgURL, exists := s.Attr("data-src")
		if !exists || imgURL == "" {
			// fallback for non-lazy images if any
			imgURL, _ = s.Attr("src")
		}

		if strings.Contains(imgURL, "lazy_placeholder") || imgURL == "" {
			return // skip placeholder
		}

		imageMap[index] = imgURL
		index++
	})

	if len(imageMap) == 0 {
		log.Printf("warning: no images found on page %s", chapterURL)
	}

	return imageMap, nil
}

// download the given image and convert to jpeg
func downloadAndConvertToJPG(imageURL, targetDir, imageName string) error {
	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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

	outputFile := filepath.Join(targetDir, imageName+".jpg")

	// If already JPEG, just save directly
	if format == "jpeg" {
		if err := os.WriteFile(outputFile, imgBytes, 0644); err != nil {
			return fmt.Errorf("failed to save jpeg image: %w", err)
		}
		return nil
	}

	// Decode image according to detected format
	var img image.Image
	switch format {
	case "png", "gif":
		img, _, err = image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode image: %w", err)
		}
	case "webp":
		img, err = webp.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode webp image: %w", err)
		}
	default:
		return fmt.Errorf("unsupported image format: %s", format)
	}

	// Convert and save as JPG
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	opts := jpeg.Options{Quality: 90}
	if err := jpeg.Encode(outFile, img, &opts); err != nil {
		return fmt.Errorf("failed to encode jpeg: %w", err)
	}

	return nil
}

// take the image number and convert it a a string to be used in the filename
func convertImgNumtoString(imgNum int) string {
	// pad the image number and convert to string
	imageNum := fmt.Sprintf("%03d", imgNum)

	return imageNum
}

func DownloadChapters() {

	// Get list of chapters already downloaded (only *.cbz)
	downloadedChapters, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("error getting file list: %v", err)
	}
	downloadedChapters = parser.FilterCBZFiles(downloadedChapters)
	log.Printf("[INFO] pmuig DownloadChapters() - chapters already downloaded: %v, from URL: https://w2.pickmeupgacha.com/", downloadedChapters)

	// Get all chapters from website
	chapterMap, err := chapterUrls()
	if err != nil {
		log.Fatalf("[ERROR] pmuig DownloadChapters() - Get Chapter URLs failed: %v", err)
	}

	// Remove already downloaded chapters from the map
	for _, chName := range downloadedChapters {
		if _, ok := chapterMap[chName]; ok {
			delete(chapterMap, chName)
			log.Printf("[INFO] pmuig DownloadChapters() - %s already downloaded, removed from chapterMap", chName)
		}
	}

	// sort the chapters
	sortedChapterList, sortError := parser.SortKeys(chapterMap)
	if sortError != nil {
		log.Fatalf("[ERROR] pmuig DownloadChapters() - failed to sort chapter keys %v", sortError)
	}

	//for _, name := range sortedChapterList {
	//	fmt.Printf("%s\n", name)
	//}
	// Start downlogin chapters
	for _, chName := range sortedChapterList {

		log.Printf("[INFO] pmuig DownloadChapters() - downloading chapter: %s", chName)

		// get the chapter image numbers and URLs
		imageMap, chapterImgError := chapterImages(chapterMap[chName])
		if chapterImgError != nil {
			log.Printf("[ERROR] pmuigDownloadChapters() - could NOT grab image list for chapter: %s", chName)
			continue
		}

		tempDir, tempDirErr := parser.CreateTempDir(chName)
		if tempDirErr != nil {
			log.Fatalf("[ERROR] pmuig DownloadChapters() - cannot create tmep dir for image download %v", tempDirErr)
		}

		// convert the image number to a string, then download image to temp dir
		for imageNum, imgUrl := range imageMap {
			imageFilename := convertImgNumtoString(imageNum)
			downloadAndConvertToJPG(imgUrl, tempDir, imageFilename)

		}
		// create CBZ file from temp dir
		parser.CreateCbzFromDir(tempDir, chName)
		fmt.Printf("created file: %s\n", chName)
	}
}
