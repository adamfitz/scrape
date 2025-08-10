package rizzfables

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"scrape/parser"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	//"github.com/chromedp/cdproto/input"

	"github.com/chai2010/webp"
	"github.com/chromedp/cdproto/network"
	"github.com/gocolly/colly"
	_ "golang.org/x/image/webp"
	_ "image/gif" // register GIF decoder
	_ "image/png" // register PNG decoder
)

// Get the chatper URLs return string slice
func chapterURLs(mangaUrl string) (map[string]string, error) {
	// resulting chapter map
	chapterMap := make(map[string]string)

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

	// Go to the list-body div, the scroll-sm unordered list and then to the list item
	c.OnHTML("div.eplister ul li", func(e *colly.HTMLElement) {
		chapterNum := e.Attr("data-num") // e.g. "7.5", "81", "42.25"
		url := e.ChildAttr("a", "href")  // from the <a> inside

		if chapterNum == "" || url == "" {
			// skip empty chapter or URL
			return
		}

		// Parse the chapterNum as float64 to validate it
		_, err := strconv.ParseFloat(chapterNum, 64)
		if err != nil {
			log.Printf("error converting chapter string to float: %v", err)
			return
		}

		// Split chapterNum into whole and fractional parts as strings
		parts := strings.Split(chapterNum, ".")

		wholePart := parts[0] // always exists
		fracPart := ""
		if len(parts) > 1 {
			fracPart = parts[1]
		}

		// Pad the whole part to 3 digits
		wholeNum, err := strconv.Atoi(wholePart)
		if err != nil {
			log.Printf("error converting whole part to int: %v", err)
			return
		}
		paddedWhole := fmt.Sprintf("%03d", wholeNum)

		// Compose final chapter name string
		var chName string
		if fracPart != "" {
			chName = fmt.Sprintf("ch%s.%s.cbz", paddedWhole, fracPart)
		} else {
			chName = fmt.Sprintf("ch%s.cbz", paddedWhole)
		}

		// Add to map
		chapterMap[chName] = url
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
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36`),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-gpu", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Enable network events
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		return nil, err
	}

	// Listen to network events, filter logs to only target site images to reduce noise
	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			url := ev.Request.URL
			if strings.Contains(url, "rizzfables.com") &&
				(strings.HasSuffix(url, ".webp") || strings.HasSuffix(url, ".jpg") || strings.HasSuffix(url, ".png")) {
				log.Printf("REQUEST: %s %s\n", ev.Request.Method, url)
			}
		case *network.EventResponseReceived:
			url := ev.Response.URL
			if strings.Contains(url, "rizzfables.com") &&
				(strings.HasSuffix(url, ".webp") || strings.HasSuffix(url, ".jpg") || strings.HasSuffix(url, ".png")) {
				log.Printf("RESPONSE: %d %s\n", ev.Response.Status, url)
			}
		}
	})

	var imageURLs []string

	jsGetImages := `
        Array.from(document.querySelectorAll('#readerarea img'))
            .map(img => img.src)
            .filter(src => src.includes('cdn.rizzfables.com/wp-content/uploads'))
    `

	err := chromedp.Run(ctx,
		chromedp.Navigate(chapterUrl),
		chromedp.WaitVisible("#readerarea img", chromedp.ByQuery),
		chromedp.Evaluate(jsGetImages, &imageURLs),
	)
	if err != nil {
		return nil, err
	}

	return imageURLs, nil
}

// DownloadAndConvertToJPG downloads an image from imageURL,
// converts to JPG if needed, and saves it inside targetDir.
// Returns error if any.
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

	opts := jpeg.Options{Quality: 90}
	err = jpeg.Encode(outFile, img, &opts)
	if err != nil {
		return fmt.Errorf("failed to encode jpeg: %w", err)
	}

	log.Printf("[SUCCESS] Converted and saved image as JPEG: %s", outputFile)

	return nil
}

// func to pad the filename to 3 digits, the inputFileName must be a filename.ext and the filename must be string
// representation of a digit.
// the input filename will be an integer.jpg (or with some image extenstion), note the input fiel name must have an
// extension
func padFileName(inputFileName string) string {
	var outputFileName string

	if strings.Contains(inputFileName, ".") {
		// split the filename on the . to separate the extension while padding
		parts := strings.SplitN(inputFileName, ".", 2)

		// convert the fielname string to an integer
		fileNamePart, err := strconv.Atoi(parts[0])
		if err != nil {
			log.Printf("padFileName() - error when converting filename integer %v", err)
		}
		// pad the resulting integer
		padded := fmt.Sprintf("%03d", fileNamePart)

		// craete the final filename
		outputFileName = padded + "." + parts[1]

	} else {
		log.Fatal("padFileName() - inputFilename must contain an extension eg: filename.ext")
	}

	return outputFileName
}

// main func to downlownd the rizzfables mangas
func DownloadMangaChapters(mangaUrl string) {
	// Get list of chapters already downloaded (only *.cbz)
	downloadedChapters, err := parser.FileList(".")
	if err != nil {
		log.Fatalf("error getting file list: %v", err)
	}
	downloadedChapters = parser.FilterCBZFiles(downloadedChapters)
	log.Printf("Chapters already downloaded: %v, from URL: %s", downloadedChapters, mangaUrl)

	// Get all chapters from website
	chapterMap, err := chapterURLs(mangaUrl)
	if err != nil {
		log.Fatalf("Get Chapter URLs failed: %v", err)
	}

	// Remove already downloaded chapters from the map
	for _, chName := range downloadedChapters {
		if _, ok := chapterMap[chName]; ok {
			delete(chapterMap, chName)
			log.Printf("%s already downloaded, removed from chapterMap", chName)
		}
	}

	// Sort chapter keys ascending (oldest to newest)
	chapterNames, err := parser.SortKeys(chapterMap)
	if err != nil {
		log.Fatal("error when sorting chapter name slice")
	}

	for _, chapter := range chapterNames {
		chapterImageURLs, err := chapterImageUrls(chapterMap[chapter])
		if err != nil {
			log.Printf("Failed to get images for chapter %s: %v", chapter, err)
			continue
		}

		chapterNum := strings.SplitN(chapter, ".", 2)[0]
		log.Printf("Starting download for chapter: %s, with %d images", chapterNum, len(chapterImageURLs))
		fmt.Printf("Starting download for chapter: %s, with %d images\n", chapterNum, len(chapterImageURLs))

		// Create temp directory for this chapter's images
		tmpDir, err := os.MkdirTemp("", "chapter-"+chapterNum)
		if err != nil {
			log.Printf("Failed to create temp dir for %s: %v", chapterNum, err)
			continue
		}
		// defer remove after archive creation below, NOT here (so it’s not removed too early)

		// Download all images in order with debug logging and correct naming
		for i, url := range chapterImageURLs {
			err := downloadAndConvertToJPG(url, tmpDir, chapterNum, i+1, len(chapterImageURLs))
			if err != nil {
				log.Printf("Error downloading image %d of chapter %s: %v", i+1, chapterNum, err)
			}
		}

		// Create CBZ from temp dir images
		targetFile := "./" + chapter
		err = parser.CreateCbzFromDir(tmpDir, targetFile)
		if err != nil {
			log.Printf("Failed to create CBZ for chapter %s: %v", chapterNum, err)
		} else {
			log.Printf("Created CBZ file: %s", targetFile)
			fmt.Printf("Created CBZ file: %s\n", targetFile)
		}

		// Clean up temp directory after CBZ creation
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf("Failed to remove temp dir %s: %v", tmpDir, err)
		}
	}
}
