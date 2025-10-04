package khinsider

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
)

// scrapeAlbum grabs only MP3 or FLAC links depending on user request
func scrapeAlbum(albumURL, format string) (map[string]string, error) {
	results := make(map[string]string)

	albumCollector := colly.NewCollector()
	trackCollector := colly.NewCollector()
	done := make(chan struct{})

	// Visit track detail pages with song name + track number context
	albumCollector.OnHTML("#songlist tr", func(e *colly.HTMLElement) {
		if e.Attr("id") == "songlist_header" {
			return
		}

		trackNum := e.ChildText("td:nth-of-type(2)") // 2nd td = track #
		songName := e.ChildText("td:nth-of-type(3) a")
		trackPage := e.ChildAttr("td:nth-of-type(3) a", "href")

		if songName != "" && trackPage != "" {
			ctx := colly.NewContext()
			ctx.Put("trackNum", trackNum)
			ctx.Put("songName", songName)
			trackCollector.Request("GET", e.Request.AbsoluteURL(trackPage), nil, ctx, nil)
		}
	})

	// Progress ticker
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				fmt.Println(" done.")
				return
			case <-ticker.C:
				fmt.Print(".")
			}
		}
	}()

	fmt.Printf("Collecting list of %s files\n", format)

	// On track page, only grab requested format
	trackCollector.OnHTML("a", func(e *colly.HTMLElement) {
		rawTrackNum := e.Request.Ctx.Get("trackNum")
		songName := e.Request.Ctx.Get("songName")
		href := e.Attr("href")
		if songName == "" || href == "" {
			return
		}

		link := e.Request.AbsoluteURL(href)
		if strings.HasSuffix(strings.ToLower(href), "."+strings.ToLower(format)) {
			// Strip non-digit chars and zero-pad
			numOnly := regexp.MustCompile(`\D`).ReplaceAllString(rawTrackNum, "")
			trackNum := rawTrackNum
			if n, err := strconv.Atoi(numOnly); err == nil {
				trackNum = fmt.Sprintf("%03d", n)
			}

			key := fmt.Sprintf("%s %s", trackNum, songName)
			results[key] = link
			log.Printf("found %s: %s -> %s", format, key, link)
		}
	})

	if err := albumCollector.Visit(albumURL); err != nil {
		return nil, err
	}

	albumCollector.Wait()
	trackCollector.Wait()
	close(done)

	return results, nil
}

func downloadFile(url, filepath string) error {
	log.Printf("[INFO] Starting download: %s -> %s", url, filepath)

	// Step 1: Create the file
	out, err := os.Create(filepath)
	if err != nil {
		log.Printf("[ERROR] Failed to create file: %s, error: %v", filepath, err)
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("[WARN] Failed to close file: %s, error: %v", filepath, cerr)
		}
	}()

	log.Printf("[INFO] File created: %s", filepath)

	// Step 2: Send HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[ERROR] Failed to GET URL: %s, error: %v", url, err)
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[WARN] Failed to close response body: %s, error: %v", url, cerr)
		}
	}()

	log.Printf("[INFO] HTTP GET successful: %s (status %s)", url, resp.Status)

	// Step 3: Check server response
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("bad status: %s", resp.Status)
		log.Printf("[ERROR] %v", err)
		return err
	}

	// Step 4: Write response body to file
	n, err := io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to write to file: %s, error: %v", filepath, err)
		return err
	}

	log.Printf("[INFO] Download completed: %s (%d bytes)", filepath, n)
	return nil
}

// SortKeys extracts the keys from a map, sorts them ascending (numerically if possible), and returns a string slice
func sortKeys(inputMap map[string]string) ([]string, error) {
	sortedList := make([]string, 0, len(inputMap))

	for key := range inputMap {
		sortedList = append(sortedList, key)
	}

	// Sort with numeric prefix if present
	sort.Slice(sortedList, func(i, j int) bool {
		// Try to extract leading track numbers
		re := regexp.MustCompile(`^(\d+)`)
		mi := re.FindStringSubmatch(sortedList[i])
		mj := re.FindStringSubmatch(sortedList[j])

		if len(mi) == 2 && len(mj) == 2 {
			ni, _ := strconv.Atoi(mi[1])
			nj, _ := strconv.Atoi(mj[1])
			return ni < nj
		}

		// Fallback to lexicographic comparison
		return sortedList[i] < sortedList[j]
	})

	return sortedList, nil
}

// sanitizeFilename removes illegal characters and trims spaces
func sanitizeFilename(name string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	clean := re.ReplaceAllString(name, "")
	clean = strings.TrimSpace(clean)
	return clean
}

// DownloadAlbum downloads all tracks and saves with zero-padded track numbers
func DownloadAlbum(url, format string) {
	log.Printf("[INFO] Starting album scrape for %s files from: %s", format, url)

	// Step 1: Scrape the album page
	songs, err := scrapeAlbum(url, format)
	if err != nil {
		log.Fatalf("[ERROR] Failed to scrape album: %v", err)
	}
	log.Printf("[INFO] Scraped %d %s tracks", len(songs), format)

	if len(songs) == 0 {
		log.Printf("[WARN] No tracks found for %s format at %s", format, url)
		return
	}

	// Step 2: Sort the map keys into a string slice
	sortedSongNames, _ := sortKeys(songs)
	log.Printf("[INFO] Sorted tracks for download")

	// Step 3: Download each file in order
	fmt.Printf("\nDownloading %s files to current dir...\n", format)
	for idx, name := range sortedSongNames {
		link := songs[name] // get value from map
		cleanName := sanitizeFilename(name)
		filePath := filepath.Join(".", cleanName+"."+format)

		// User sees only clean download message
		fmt.Printf("Downloading: %s\n", cleanName)

		// Full HTTP info logged, not printed to user
		log.Printf("[INFO] (%d/%d) Track: %s, URL: %s, Saving to: %s",
			idx+1, len(sortedSongNames), cleanName, link, filePath)

		if err := downloadFile(link, filePath); err != nil {
			log.Printf("[ERROR] Failed to download %s: %v", cleanName, err)
		} else {
			log.Printf("[INFO] Successfully downloaded: %s", cleanName)
		}
	}

	log.Printf("[INFO] All downloads completed")
}
