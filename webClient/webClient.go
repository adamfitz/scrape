package webClient

import (
	"bytes"
	"fmt"
	"github.com/gocolly/colly"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

// NewHTTPClient returns a new HTTP client with a cookie jar
func NewHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
	}
}

// NewImageRequest creates a new HTTP GET request for an image, with common anti-bot headers
func NewImageRequest(url string, referer string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", referer)

	return req, nil
}

// FetchImageBytes fetches image data, checks HTTP status and HTML error pages, and retries on 5xx errors
func FetchImageBytes(client *http.Client, req *http.Request) ([]byte, error) {
	const maxRetries = 3
	const retryDelay = 2 * time.Second

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to fetch image %s: %v", req.URL.String(), err)
			time.Sleep(retryDelay)
			continue
		}

		// Check for retry-worthy HTTP status codes
		if resp.StatusCode >= 500 && resp.StatusCode < 600 || resp.StatusCode == 525 {
			lastErr = fmt.Errorf("received HTTP %d for %s", resp.StatusCode, req.URL.String())
			resp.Body.Close()
			time.Sleep(retryDelay)
			continue
		}

		// Check OK
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected HTTP status: %d for %s", resp.StatusCode, req.URL.String())
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read image body %s: %v", req.URL.String(), err)
		}

		if bytes.HasPrefix(bytes.TrimSpace(bodyBytes), []byte("<!DOCTYPE")) ||
			bytes.HasPrefix(bytes.TrimSpace(bodyBytes), []byte("<html")) {
			return nil, fmt.Errorf("received HTML instead of image for %s", req.URL.String())
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			return nil, fmt.Errorf("unexpected Content-Type: %s for %s", contentType, req.URL.String())
		}

		// Success
		return bodyBytes, nil
	}

	return nil, fmt.Errorf("failed after %d attempts: %v", maxRetries, lastErr)
}

// Implements an exponential backoff up to 320s. If at 320s and still failing, it will retry 3 times then hard fail.
// Successful fetch halves the backoff and resets the max-backoff retry counter.
// Returns the response body or an error if all retries fail.
func FetchWithBackoff(client *http.Client, req *http.Request) ([]byte, error) {
	const (
		initialBackoff  = 10 * time.Second
		maxBackoff      = 320 * time.Second
		maxRetriesAtMax = 3
	)

	backoff := initialBackoff
	retriesAtMax := 0
	attempt := 1

	for {
		resp, err := client.Do(req)
		if err != nil {
			if os.IsTimeout(err) || strings.Contains(err.Error(), "Client.Timeout") {
				log.Printf("Attempt %d: Timeout fetching %s: %v. Backing off for %v", attempt, req.URL, err, backoff)
				time.Sleep(backoff)

				if backoff < maxBackoff {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				} else {
					retriesAtMax++
					if retriesAtMax >= maxRetriesAtMax {
						return nil, fmt.Errorf("failed after %d retries at max backoff (%v) for %s", maxRetriesAtMax, maxBackoff, req.URL)
					}
				}
				attempt++
				continue
			}
			return nil, err
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		// On success, halve backoff and reset max retry counter
		if backoff > initialBackoff {
			backoff /= 2
			if backoff < initialBackoff {
				backoff = initialBackoff
			}
		}
		retriesAtMax = 0

		log.Printf("Attempt %d: Successfully fetched %s", attempt, req.URL)
		return body, nil
	}
}

// Implements an exponential backoff and logs errors when fetching the chapter page HTML
func FetchChapterPage(chapterURL string) (string, error) {
	const (
		initialBackoff  = 10 * time.Second
		maxBackoff      = 320 * time.Second
		maxRetriesAtMax = 3
	)

	backoff := initialBackoff
	retriesAtMax := 0
	attempt := 1

	var pageHTML string

	for {
		c := colly.NewCollector()
		c.SetRequestTimeout(60 * time.Second) // increase timeout for slow pages

		// Capture the full HTML
		c.OnResponse(func(r *colly.Response) {
			pageHTML = string(r.Body)
		})

		err := c.Visit(chapterURL)
		if err == nil && strings.TrimSpace(pageHTML) != "" {
			log.Printf("Attempt %d: Successfully fetched chapter page %s", attempt, chapterURL)
			return pageHTML, nil
		}

		// Log error and backoff
		log.Printf("Attempt %d: Failed to fetch chapter page %s: %v. Backing off %v", attempt, chapterURL, err, backoff)
		time.Sleep(backoff)

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			retriesAtMax++
			if retriesAtMax >= maxRetriesAtMax {
				return "", fmt.Errorf("failed after %d retries at max backoff (%v) for chapter page %s", maxRetriesAtMax, maxBackoff, chapterURL)
			}
		}

		attempt++
	}
}
