package commands

import (
	"fmt"
	"log"
	"os"
	"scrape/asura"
	"scrape/cfotz"
	"scrape/cfproxy"
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
	"time"

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
	Long:  `Download manga chapters from KunManga website using shortname with optional Cloudflare bypass`,
	Run: func(cmd *cobra.Command, args []string) {
		shortName, _ := cmd.Flags().GetString("shortname")
		start, _ := cmd.Flags().GetInt("start")
		end, _ := cmd.Flags().GetInt("end")
		useProxy, _ := cmd.Flags().GetBool("bypass-cf")
		proxyPort, _ := cmd.Flags().GetInt("proxy-port")

		if shortName == "" {
			fmt.Println("Error: --shortname flag is required")
			cmd.Usage()
			os.Exit(1)
		}

		// Start CF proxy if requested
		var proxy *cfproxy.ProxyServer
		if useProxy {
			fmt.Println("🚀 Starting Cloudflare bypass proxy...")
			proxy = cfproxy.NewProxyServer(proxyPort)
			err := proxy.Start()
			if err != nil {
				fmt.Printf("❌ Failed to start proxy: %v\n", err)
				os.Exit(1)
			}

			// Give proxy time to start
			time.Sleep(2 * time.Second)

			// Test the proxy with a KunManga URL
			testURL := fmt.Sprintf("https://kunmanga.com/manga/%s", shortName)
			fmt.Printf("🧪 Testing proxy with %s...\n", testURL)
			if err := cfproxy.TestFetch(proxyPort, testURL); err != nil {
				fmt.Printf("❌ Proxy test failed: %v\n", err)
				proxy.Stop()
				os.Exit(1)
			}

			fmt.Println("✅ Cloudflare bypass proxy ready!")

			// Ensure cleanup on exit
			defer func() {
				fmt.Println("🛑 Stopping Cloudflare bypass proxy...")
				proxy.Stop()
			}()
		}

		type Chapter struct {
			URL    string
			Slug   string
			Number int
		}

		// Get chapter URLs - use proxy if enabled
		var chapterURLs []string
		if useProxy {
			fmt.Println("📡 Fetching chapter list through CF proxy...")
			chapterURLs = kunmanga.KunMangaChapterUrlsWithProxy(shortName, proxyPort)
		} else {
			chapterURLs = kunmanga.KunMangaChapterUrls(shortName)
		}

		if len(chapterURLs) == 0 {
			fmt.Println("⚠️  No chapters found for manga:", shortName)
			os.Exit(1)
		}

		fmt.Printf("📚 Found %d chapters for %s\n", len(chapterURLs), shortName)

		var filtered []Chapter

		for _, chapterUrl := range chapterURLs {
			parts := strings.Split(strings.Trim(chapterUrl, "/"), "/")
			chapterSlug := parts[len(parts)-1]
			chNum := kunmanga.ParseChapterNumber(chapterSlug)

			// Filter by range if specified
			if start != 0 && end != 0 {
				if chNum < start || chNum > end {
					log.Printf("[FILTER] Skipping chapter %s (number %d): outside range %d-%d", chapterSlug, chNum, start, end)
					continue
				}
			}

			// Check if CBZ already exists
			exists, err := parser.CBZExists(chNum)
			if err != nil {
				log.Printf("[ERROR] Error checking CBZ for chapter %d: %v", chNum, err)
				continue
			}
			if exists {
				log.Printf("[SKIP] Chapter %d already exists", chNum)
				continue
			}

			filtered = append(filtered, Chapter{
				URL:    chapterUrl,
				Slug:   chapterSlug,
				Number: chNum,
			})
		}

		if len(filtered) == 0 {
			fmt.Println("ℹ️  No new chapters to download (all exist or filtered out)")
			return
		}

		// Sort chapters by number (ascending)
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Number < filtered[j].Number
		})

		fmt.Printf("📥 Will download %d chapters\n", len(filtered))

		// Download each chapter
		for i, ch := range filtered {
			fmt.Printf("📖 [%d/%d] Downloading chapter %d (%s)...\n",
				i+1, len(filtered), ch.Number, ch.Slug)

			var err error
			if useProxy {
				// Use proxy-enabled download
				err = kunmanga.DownloadKunMangaChaptersWithProxy(ch.URL, ch.Number, proxyPort)
			} else {
				// Use regular download
				err = kunmanga.DownloadKunMangaChapters(ch.URL, ch.Number)
			}

			if err != nil {
				log.Printf("[ERROR] Failed to download chapter %s: %v", ch.Slug, err)
				fmt.Printf("❌ Failed to download chapter %d, continuing...\n", ch.Number)
				continue
			}

			fmt.Printf("✅ Successfully downloaded chapter %d\n", ch.Number)
		}

		fmt.Printf("🎉 Completed! Downloaded %d chapters for %s\n", len(filtered), shortName)
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

	// New CF proxy flags
	kunmangaCmd.Flags().Bool("bypass-cf", false, "Use Cloudflare bypass proxy")
	kunmangaCmd.Flags().Int("proxy-port", 23181, "Port for the Cloudflare bypass proxy")

	xbatoCmd.Flags().String("shortname", "", "Shortname for the manga (required)")
}
