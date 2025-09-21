package commands

import (
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

	"github.com/spf13/cobra"
)

// ManhuaUS command
var manhuausCmd = &cobra.Command{
	Use:   "manhuaus",
	Short: "Scrape chapters from ManhuaUS",
	Long:  `Download manga chapters from ManhuaUS website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		chapterList, err := manhuaus.ChapterURLs(url)
		if err != nil {
			fmt.Printf("%s\nError retrieving chapter list from manhuaus\n", err)
			os.Exit(1)
		}

		toDownload := parser.FilterUndownloadedChapters(chapterList)
		totalChapters := len(toDownload)
		fmt.Println("Downloading", totalChapters, "chapters")

		for _, chapter := range toDownload {
			chNumber, err := manhuaus.ExtractChapterNumber(chapter)
			if err != nil {
				fmt.Printf("%s\nError extracting chapter number from URL\n", err)
				continue
			}
			fmt.Println("Downloading chapter #", chNumber)
			err = manhuaus.DownloadChaper(chapter, chNumber)
			if err != nil {
				fmt.Printf("%s\nError downloading chapter number: %s\n", err, chapter)
				os.Exit(1)
			}
		}
	},
}

// KunManga command
var kunmangaCmd = &cobra.Command{
	Use:   "kunmanga",
	Short: "Scrape chapters from KunManga",
	Long:  `Download manga chapters from KunManga website using shortname`,
	Run: func(cmd *cobra.Command, args []string) {
		shortName, _ := cmd.Flags().GetString("shortname")
		start, _ := cmd.Flags().GetInt("start")
		end, _ := cmd.Flags().GetInt("end")

		if shortName == "" {
			fmt.Println("Error: --shortname flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		type Chapter struct {
			URL    string
			Slug   string
			Number int
		}

		chapterURLs := kunmanga.KunMangaChapterUrls(shortName)
		var filtered []Chapter

		for _, chapterUrl := range chapterURLs {
			parts := strings.Split(strings.Trim(chapterUrl, "/"), "/")
			chapterSlug := parts[len(parts)-1]
			chNum := kunmanga.ParseChapterNumber(chapterSlug)

			if start != 0 && end != 0 {
				if chNum < start || chNum > end {
					log.Printf("[MAIN] Skipping chapter %s (number %d): outside range %d-%d", chapterSlug, chNum, start, end)
					continue
				}
			}

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

		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Number < filtered[j].Number
		})

		for _, ch := range filtered {
			fmt.Printf("Downloading chapter %d (%s)...\n", ch.Number, ch.Slug)
			err := kunmanga.DownloadKunMangaChapters(ch.URL, ch.Number)
			if err != nil {
				log.Printf("[MAIN] Error downloading chapter %s: %v", ch.Slug, err)
			}
		}
	},
}

// Xbato command
var xbatoCmd = &cobra.Command{
	Use:   "xbato",
	Short: "Scrape chapters from Xbato",
	Long:  `Download manga chapters from Xbato website using shortname`,
	Run: func(cmd *cobra.Command, args []string) {
		shortName, _ := cmd.Flags().GetString("shortname")

		if shortName == "" {
			fmt.Println("Error: --shortname flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		parser.CheckBrowser("xbato")

		chapterUrls, err := xbato.XbatoChapterUrls(shortName)
		if err != nil {
			fmt.Printf("%s\nError retrieving chapter list from xbato\n", err)
			os.Exit(1)
		}

		chapterMap, err := xbato.ChapterOptions(chapterUrls[0])
		if err != nil {
			fmt.Println("error retrieving chapterMap from url: ", err)
			os.Exit(1)
		}

		formattedChapterMap := xbato.FormatChapterMap(chapterMap)
		xbato.DownloadAndCreateCBZ(chapterUrls, formattedChapterMap)
	},
}

// Iluim command
var iluimCmd = &cobra.Command{
	Use:   "iluim",
	Short: "Scrape chapters from Infinite Level Up",
	Long:  `Download manga chapters from Infinite Level Up website`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Starting iluim scraper...")
		chapterUrls, err := iluim.ChapterURLs("https://infinitelevelup.com/")
		if err != nil {
			log.Fatalf("Get Chapter URLs failed: %v", err)
		}
		iluim.DownloadChapters(chapterUrls)
	},
}

// ORV command
var orvCmd = &cobra.Command{
	Use:   "orv",
	Short: "Scrape ORV chapters",
	Long:  `Download missing ORV manga chapters`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("ORV starting download of missing ORV chapters...")
		orv.DownloadMangaChapters()
	},
}

// Rizzfables command
var rizzfablesCmd = &cobra.Command{
	Use:   "rizzfables",
	Short: "Scrape chapters from Rizzfables",
	Long:  `Download manga chapters from Rizzfables website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		fmt.Println("Rizzfables starting chapter download...")
		rizzfables.DownloadMangaChapters(url)
	},
}

// HLS command
var hlsCmd = &cobra.Command{
	Use:   "hls",
	Short: "Scrape Honey Lemon Soda chapters",
	Long:  `Download Honey Lemon Soda manga chapters`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Honey Lemon Soda, starting chapter download...")
		hls.DownloadChapters()
	},
}

// Mgeko command
var mgekoCmd = &cobra.Command{
	Use:   "mgeko",
	Short: "Scrape chapters from Mgeko",
	Long:  `Download manga chapters from Mgeko website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		targetName := parser.MgekoUrlToName(url)
		fmt.Printf("%s, starting chapter download...\n", targetName)
		mgeko.DownloadChapters(url)
	},
}

// Cfotz command
var cfotzCmd = &cobra.Command{
	Use:   "cfotz",
	Short: "Scrape Childhood Friend of the Zenith chapters",
	Long:  `Download Childhood Friend of the Zenith manga chapters`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Starting download childhood friend of the zenith\n")
		cfotz.DownloadChapters()
	},
}

// Stonescape command
var stonescapeCmd = &cobra.Command{
	Use:   "stonescape",
	Short: "Scrape chapters from Stonescape",
	Long:  `Download manga chapters from Stonescape website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		fmt.Printf("Starting download from stonescape for: %s\n", url)
		stonescape.DownloadChapters(url)
	},
}

// Asura command
var asuraCmd = &cobra.Command{
	Use:   "asura",
	Short: "Scrape chapters from AsuraComics",
	Long:  `Download manga chapters from AsuraComics website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		fmt.Printf("Starting download from asuracomics for: %s\n", url)
		asura.DownloadChapters(url)
	},
}

// Ravenscans command
var ravenscansCmd = &cobra.Command{
	Use:   "ravenscans",
	Short: "Scrape chapters from RavenScans",
	Long:  `Download manga chapters from RavenScans website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		fmt.Printf("Starting download from ravenscans for: %s\n", url)
		ravenscans.DownloadMangaChapters(url)
	},
}

func init() {
	// Add flags to commands that need them
	manhuausCmd.Flags().String("url", "", "Chapter URL to scrape (required)")
	rizzfablesCmd.Flags().String("url", "", "Chapter URL to scrape (required)")
	mgekoCmd.Flags().String("url", "", "Chapter URL to scrape (required)")
	stonescapeCmd.Flags().String("url", "", "Chapter URL to scrape (required)")
	asuraCmd.Flags().String("url", "", "Chapter URL to scrape (required)")
	ravenscansCmd.Flags().String("url", "", "Chapter URL to scrape (required)")

	kunmangaCmd.Flags().String("shortname", "", "Shortname for the manga (required)")
	kunmangaCmd.Flags().Int("start", 0, "Start chapter number (optional)")
	kunmangaCmd.Flags().Int("end", 0, "End chapter number (optional)")

	xbatoCmd.Flags().String("shortname", "", "Shortname for the manga (required)")
}
