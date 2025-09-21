package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"scrape/asura"
	"scrape/cfotz"
	"scrape/hls"
	"scrape/iluim"
	"scrape/kunmanga"
	"scrape/manhuaus"
	"scrape/mgeko"
	"scrape/orv"
	"scrape/parser"
	"scrape/ravenscans"
	"scrape/rizzfables"
	"scrape/stonescape"
	"scrape/xbato"
	"sort"
	"strings"
)

func init() {
	logFile, err := os.OpenFile("/var/log/scrape/scrape.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
		fmt.Printf("Cannot open log file, check /var/log/scrape/scrape.log permissions/\n%v", err)
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

	if *urlFlag == "" && *siteName == "rizzfables" {
		fmt.Println("Error: --url flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *urlFlag == "" && *siteName == "mgeko" {
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
	if *urlFlag == "" && *siteName == "stonescape" {
		fmt.Println("Error: --url flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *urlFlag == "" && *siteName == "asura" {
		fmt.Println("Error: --url flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *urlFlag == "" && *siteName == "ravenscans" {
		fmt.Println("Error: --url flag is required")
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
			// check if th chapter already exists or not
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

		// check chrome is installed (required)
		parser.CheckBrowser("xbato")

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
	case "orv":
		log.Println("ORV starting download of missing ORV chapters...")
		orv.DownloadMangaChapters()
	case "rizzfables":
		fmt.Println("Rizzfables starting chapter download...")
		rizzfables.DownloadMangaChapters(*urlFlag)
	case "hls":
		fmt.Println("Honey Lemon Soda, starting chapter download...")
		hls.DownloadChapters()
	case "mgeko":
		targetName := parser.MgekoUrlToName(*urlFlag)
		fmt.Printf("%s, starting chapter download...\n", targetName)

		mgeko.DownloadChapters(*urlFlag)
	case "cfotz":
		fmt.Printf("Starting download childhood friend of the zenith\n")
		cfotz.DownloadChapters()
	case "stonescape":
		fmt.Printf("Starting download from stonescape for: %s\n", *urlFlag)
		stonescape.DownloadChapters(*urlFlag)
	case "asura":
		fmt.Printf("Starting download from asuracomics for: %s\n", *urlFlag)
		asura.DownloadChapters(*urlFlag)
	case "ravenscans":
		fmt.Printf("Starting download from ravenscans for: %s\n", *urlFlag)
		ravenscans.DownloadMangaChapters(*urlFlag)
	}
}
