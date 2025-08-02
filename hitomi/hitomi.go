package hitomi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"encoding/json"

	"github.com/chai2010/webp"
	"github.com/chromedp/chromedp"

	"scrape/parser"
	cdplog "github.com/chromedp/cdproto/log"
    cdpruntime "github.com/chromedp/cdproto/runtime"
    stdlog "log"
)

// extractGalleryID extracts numeric gallery ID from a URL string.
func extractGalleryID(url string) (string, error) {
	re := regexp.MustCompile(`-([0-9]+)\.html`)
	matches := re.FindStringSubmatch(url)
	if len(matches) != 2 {
		return "", errors.New("could not extract gallery ID from URL")
	}
	return matches[1], nil
}

// GetImageURLsViaChromedp navigates to the reader page and extracts all image URLs from #comicImages img tags.
func GetImageURLsViaChromedp(galleryID string) ([]string, error) {
    opts := append(chromedp.DefaultExecAllocatorOptions[:],
        chromedp.Flag("headless", true),
        chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; x64) AppleWebKit/537.36 Chrome/117.0.0.0 Safari/537.36"),
    )

    allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
    defer cancelAlloc()

    ctx, cancelCtx := chromedp.NewContext(allocCtx,
        chromedp.WithDebugf(stdlog.Printf), // Enable chromedp debug logs
    )
    defer cancelCtx()

    // Listen for browser console events
    chromedp.ListenTarget(ctx, func(ev interface{}) {
        switch ev := ev.(type) {
        case *cdpruntime.EventConsoleAPICalled:
            for _, arg := range ev.Args {
                stdlog.Printf("[Browser Console] %s\n", arg.Value)
            }
        case *cdplog.EventEntryAdded:
            stdlog.Printf("[Browser Log] %s\n", ev.Entry.Text)
        }
    })

    ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
    defer cancelTimeout()

    url := fmt.Sprintf("https://hitomi.la/reader/%s.html", galleryID)

    var imageURLs []string
    var htmlContent string

    err := chromedp.Run(ctx,
        chromedp.Navigate(url),
        chromedp.WaitVisible(`#comicImages`, chromedp.ByID),
        chromedp.Sleep(2*time.Second),

        // Scroll to force lazy loading of all images
        chromedp.ActionFunc(func(ctx context.Context) error {
            for i := 0; i < 10; i++ {
                if err := chromedp.Evaluate(`window.scrollBy(0, window.innerHeight)`, nil).Do(ctx); err != nil {
                    return err
                }
                time.Sleep(1 * time.Second) // wait for images to load
            }
            return nil
        }),

        chromedp.Sleep(2*time.Second),

        // Get full innerHTML of #comicImages container for debugging
        chromedp.InnerHTML(`#comicImages`, &htmlContent, chromedp.ByID),

        // Extract image srcs after scrolling
        chromedp.Evaluate(`Array.from(document.querySelectorAll("#comicImages img")).map(img => img.src)`, &imageURLs),
    )
    if err != nil {
        return nil, fmt.Errorf("chromedp error extracting image URLs: %w", err)
    }

    stdlog.Printf("[DEBUG] #comicImages innerHTML length: %d\n", len(htmlContent))
    // Optionally save the HTML snapshot to a file for inspection:
    _ = os.WriteFile("debug_comicimages.html", []byte(htmlContent), 0644)

    if len(imageURLs) == 0 {
        return nil, errors.New("no image URLs found on reader page")
    }

    stdlog.Printf("[DEBUG] Found %d images\n", len(imageURLs))
    for i, imgURL := range imageURLs {
        stdlog.Printf("  %03d: %s\n", i+1, imgURL)
    }

    return imageURLs, nil
}




// DownloadHitomiOneshot downloads all images for a gallery URL and creates a CBZ archive.
func DownloadHitomiOneshot(fullURL, outputName string) error {
	tmpDir := "tmp_hitomi"
	if err := os.MkdirAll(tmpDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	galleryID, err := extractGalleryID(fullURL)
	if err != nil {
		return fmt.Errorf("failed to extract gallery ID: %w", err)
	}
	fmt.Println("[INFO] Gallery ID:", galleryID)

	imageURLs, err := GetImageURLsViaChromedp(galleryID)
	if err != nil {
		return fmt.Errorf("failed to get image URLs: %w", err)
	}

	client := &http.Client{}

	for i, imgURL := range imageURLs {
		fmt.Printf("[INFO] Downloading %03d: %s\n", i+1, imgURL)

		req, err := http.NewRequest("GET", imgURL, nil)
		if err != nil {
			fmt.Printf("[WARN] Failed to create request for %s: %v\n", imgURL, err)
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/117.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://hitomi.la/")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[WARN] HTTP request failed for %s: %v\n", imgURL, err)
			continue
		}

		if resp.StatusCode != 200 {
			fmt.Printf("[WARN] Non-200 HTTP status %d for %s\n", resp.StatusCode, imgURL)
			resp.Body.Close()
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			debugPath := filepath.Join(tmpDir, fmt.Sprintf("%03d_response.html", i+1))
			os.WriteFile(debugPath, bodyBytes, 0644)
			fmt.Printf("[WARN] Non-image content (%s) dumped to %s\n", contentType, debugPath)
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("[WARN] Failed to read image body %s: %v\n", imgURL, err)
			continue
		}

		imgReader := bytes.NewReader(bodyBytes)
		var format string
		img, f, err := image.Decode(imgReader)
		if err != nil {
			// Try webp fallback
			imgReader.Seek(0, io.SeekStart)
			img, err = webp.Decode(imgReader)
			if err != nil {
				fmt.Printf("[WARN] Failed to decode image %s: %v\n", imgURL, err)
				continue
			}
			format = "webp"
		} else {
			format = f
		}

		fmt.Printf("[DEBUG] Decoded image format: %s\n", format)

		imgPath := filepath.Join(tmpDir, fmt.Sprintf("%03d.jpg", i+1))
		outFile, err := os.Create(imgPath)
		if err != nil {
			fmt.Printf("[WARN] Failed to create file %s: %v\n", imgPath, err)
			continue
		}

		err = jpeg.Encode(outFile, img, &jpeg.Options{Quality: 90})
		outFile.Close()
		if err != nil {
			fmt.Printf("[WARN] Failed to encode JPEG %s: %v\n", imgPath, err)
			continue
		}
	}

	cbzName := outputName + ".cbz"
	fmt.Println("[INFO] Creating CBZ:", cbzName)

	if err := parser.CreateCBZ(tmpDir, cbzName); err != nil {
		return fmt.Errorf("failed to create CBZ: %w", err)
	}

	if err := os.RemoveAll(tmpDir); err != nil {
		fmt.Printf("[WARN] Failed to clean up temp directory: %v\n", err)
	}

	fmt.Println("[SUCCESS] CBZ created:", cbzName)
	return nil
}


// GalleryMetadata represents the main structure inside the galleries/{id}.js file.
// For now, we define it loosely as a map[string]interface{} to print raw data.
type GalleryMetadata map[string]any

// FetchGalleryMetadata fetches and parses the gallery metadata JS file from Hitomi.la and prints the JSON content.
func FetchGalleryMetadataFromReader(galleryID string) error {
	url := fmt.Sprintf("https://hitomi.la/reader/%s.html", galleryID)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/117.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var galleryinfoJSON string

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(2*time.Second),

		// This extracts galleryinfo as a JSON string
		chromedp.Evaluate(`JSON.stringify(galleryinfo)`, &galleryinfoJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to evaluate galleryinfo: %w", err)
	}

	// Parse the JSON string
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(galleryinfoJSON), &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal galleryinfo: %w", err)
	}

	// Pretty print the metadata
	formatted, _ := json.MarshalIndent(metadata, "", "  ")
	fmt.Println(string(formatted))

	return nil
}

type GalleryInfo struct {
	Files []struct {
		Hash string `json:"hash"`
		Name string `json:"name"`
	} `json:"files"`
}

// BuildImageURLsFromGalleryJSON parses the gallery JSON and constructs full image URLs.
func BuildImageURLsFromGalleryJSON(jsonStr string) ([]string, error) {
	var info GalleryInfo
	if err := json.Unmarshal([]byte(jsonStr), &info); err != nil {
		return nil, fmt.Errorf("failed to parse galleryinfo JSON: %w", err)
	}

	var urls []string
	for _, file := range info.Files {
		subdomain := "a" // optionally hash-based shard logic here
		url := fmt.Sprintf("https://%s.hitomi.la/webp/%s.webp", subdomain, file.Hash)
		urls = append(urls, url)
	}

	return urls, nil
}


