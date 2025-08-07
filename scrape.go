package main

import (
	"flag"
	"fmt"
	//"go/parser"
	"log"
	"os"
	"scrape/iluim"
	"scrape/kunmanga"
	"scrape/manhuaus"
	"scrape/parser"
	"scrape/xbato"
	"sort"
	"strings"

	_ "golang.org/x/image/webp" // Add support for decoding webp
)

func init() {
	logFile, err := os.OpenFile("scrape.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
}

func main() {
	siteName := flag.String("site", "", "name of website")
	urlFlag := flag.String("url", "", "Chapter URL to scrape (required)")
	shortName := flag.String("shortname", "", "Kunmanga/Xbato shortname is required")
	start := flag.Int("start", 0, "Start chapter number (optional)")
	end := flag.Int("end", 0, "End chapter number (optional)")
	flag.Parse()

	if *urlFlag == "" && *siteName == "manhuaus" {
		fmt.Println("Error: --url flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *shortName == "" && *siteName == "kunmanga" {
		fmt.Println("Error: --shortname flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *shortName == "" && *siteName == "xbato" {
		fmt.Println("Error: --shortname flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *siteName == "" {
		fmt.Println("Error: --site flag is required")
		flag.Usage()
		os.Exit(1)
	}
	switch *siteName { // MUST dereference pointer before comparison in swtich
	case "manhuaus":
		chapterList, err := manhuaus.ChapterURLs(*urlFlag)
		if err != nil {
			fmt.Printf("%s\nError retrieving chapter list from %s", err, *siteName)
			os.Exit(1)
		}

		// remove any chapters that have already been downloaded (if they are in the current dir)
		toDownload := parser.FilterUndownloadedChapters(chapterList)

		totalChapters := len(toDownload)
		fmt.Println("Downloading", totalChapters, "chapters")

		for _, chapter := range toDownload {

			chNumber, err := manhuaus.ExtractChapterNumber(chapter)
			if err != nil {
				fmt.Printf("%s\nError extracting chapter number from URL", err)
				continue
			}
			// check if teh chapter already exists or not
			fmt.Println("Downloading chapter #", chNumber)
			err = manhuaus.DownloadChaper(chapter, chNumber)

			if err != nil {
				fmt.Printf("%s\nError downloading chapter number: %s", err, chapter)
				os.Exit(1)
			}
		}
	case "kunmanga":
		// Chapter holds URL and parsed chapter number
		type Chapter struct {
			URL    string
			Slug   string
			Number int
		}

		chapterURLs := kunmanga.KunMangaChapterUrls(*shortName)

		var filtered []Chapter

		// Filter and parse chapters on the fly
		for _, chapterUrl := range chapterURLs {
			parts := strings.Split(strings.Trim(chapterUrl, "/"), "/")
			chapterSlug := parts[len(parts)-1]

			chNum := kunmanga.ParseChapterNumber(chapterSlug)
			/* I dont think this is needed as regardless of the number all chapter links are required.
			if chNum < 0 { // sometimes there is a chapter-0 eg: prologue etc, so we want to start at zero
				log.Printf("[MAIN] Skipping invalid chapter slug: %s", chapterSlug)
				continue
			}
			*/

			// Only filter by range if both start and end are set
			if *start != 0 && *end != 0 {
				if chNum < *start || chNum > *end {
					log.Printf("[MAIN] Skipping chapter %s (number %d): outside range %d-%d", chapterSlug, chNum, *start, *end)
					continue
				}
			}

			// Check if CBZ already exists
			exists, err := parser.CBZExists(chNum)
			if err != nil {
				log.Printf("[MAIN] Kunmanga error checking CBZ for chapter %d: %v", chNum, err)
				continue
			}
			if exists {
				log.Printf("[MAIN] Kunmanga skipping chapter %d: CBZ already exists", chNum)
				continue
			}

			filtered = append(filtered, Chapter{
				URL:    chapterUrl,
				Slug:   chapterSlug,
				Number: chNum,
			})
		}

		// Sort filtered chapters ascending by chapter number
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Number < filtered[j].Number
		})

		// Download each filtered chapter
		for _, ch := range filtered {
			//log.Printf("[MAIN] Starting download for chapter %s (number %d)", ch.Slug, ch.Number)
			fmt.Printf("Downloading chapter %d (%s)...\n", ch.Number, ch.Slug)
			err := kunmanga.DownloadKunMangaChapters(ch.URL, ch.Number)
			if err != nil {
				log.Printf("[MAIN] Error downloading chapter %s: %v", ch.Slug, err)
			}
		}
	case "xbato":
		chapterUrls, err := xbato.XbatoChapterUrls(*shortName)
		if err != nil {
			fmt.Printf("%s\nError retrieving chapter list from %s", err, *siteName)
			os.Exit(1)
		}

		// grab the list of chapters from the
		chapterMap, err := xbato.ChapterOptions(chapterUrls[0])
		if err != nil {
			fmt.Println("error retrieving chapterMap from url: ", err)
			os.Exit(1)
		}

		// format the chapter names
		formattedChapterMap := xbato.FormatChapterMap(chapterMap)

		// download the chapters
		xbato.DownloadAndCreateCBZ(chapterUrls, formattedChapterMap)

	case "iluim":
		log.Println("Starting iluim scraper...")
		chapterUrls, err := iluim.ChapterURLs("https://infinitelevelup.com/")
		if err != nil {
			log.Fatalf("Get Chapter URls failed: %v", err)
		}
		iluim.DownloadChapters(chapterUrls)
	}
}
