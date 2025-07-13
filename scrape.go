package main

import (
	"flag"
	"fmt"
	_ "golang.org/x/image/webp" // Add support for decoding webp
	"log"
	"os"
	"scrape/kunmanga"
	"scrape/manhuaus"
	"scrape/xbato"
	"sort"
	"strings"
)

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

		sortedChapters, err := manhuaus.SortAndFilterChapters(chapterList, *start, *end)
		if err != nil {
			fmt.Printf("%s\nError sorting chapter list", err)
			os.Exit(1)
		}

		totalChapters := len(sortedChapters)
		fmt.Println("Downloading", totalChapters, "chapters")

		for _, chapter := range sortedChapters {
			chNumber, err := manhuaus.ExtractChapterNumber(chapter)
			if err != nil {
				fmt.Printf("%s\nError extracting chapter number from URL", err)
				continue
			}
			fmt.Println("Downloading chapter #", chNumber)
			err = manhuaus.DownloadChaper(chapter)
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
			if chNum == 0 {
				log.Printf("[MAIN] Skipping invalid chapter slug: %s", chapterSlug)
				continue
			}

			// Only filter by range if both start and end are set
			if *start != 0 && *end != 0 {
				if chNum < *start || chNum > *end {
					log.Printf("[MAIN] Skipping chapter %s (number %d): outside range %d-%d", chapterSlug, chNum, *start, *end)
					continue
				}
			}

			// Check if CBZ already exists
			exists, err := kunmanga.CBZExists(chNum)
			if err != nil {
				log.Printf("[MAIN] Error checking CBZ for chapter %d: %v", chNum, err)
				continue
			}
			if exists {
				log.Printf("[MAIN] Skipping chapter %d: CBZ already exists", chNum)
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
		chapterList, err := xbato.XbatoChapterUrls(*shortName)
		if err != nil {
			fmt.Printf("%s\nError retrieving chapter list from %s", err, *siteName)
			os.Exit(1)
		}
		for _, element := range chapterList {
			fmt.Println(element)
		}
	}
}
