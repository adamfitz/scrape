package cf

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"runtime"
	"fmt"
	"time"
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func FindChromeExec() (string, error) {
	var candidates []string

	switch runtime.GOOS {
	case "linux":
		// prefer Google Chrome installed via .deb or system package
		candidates = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chrome",
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
		}

		// Skip Snap paths explicitly
		snapPaths := []string{
			"/snap/bin/chromium",
		}

		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				// make sure it's not a Snap
				isSnap := false
				for _, sp := range snapPaths {
					if path == sp {
						isSnap = true
						break
					}
				}
				if !isSnap {
					log.Printf("Chrome binary found at: %s", path)
					return path, nil
				}
			}
		}

	case "darwin":
		paths := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				log.Printf("Chrome binary found at: %s", p)
				return p, nil
			}
		}
		candidates = []string{"google-chrome", "chrome", "chromium"}

	case "windows":
		progs := []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Chromium\Application\chrome.exe`,
			`C:\Program Files (x86)\Chromium\Application\chrome.exe`,
		}
		for _, p := range progs {
			if _, err := os.Stat(p); err == nil {
				log.Printf("Chrome binary found at: %s", p)
				return p, nil
			}
		}
		candidates = []string{"chrome.exe", "chrome"}

	default:
		return "", errors.New("unsupported platform")
	}

	// fallback: search PATH
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			// skip snap
			if !strings.HasPrefix(path, "/snap/") {
				log.Printf("Chrome binary found in PATH: %s", path)
				return path, nil
			}
		}
	}

	return "", errors.New("chrome/chromium not found in PATH or common locations (non-snap only)")
}



// SpawnChromeAndCollectCFCookies starts a visible Chrome process (chromedp) using the provided
// chromePath binary and profileDir (if empty a temp profile is created).
// It navigates to targetURL, waits until one or more cookies whose names start with "cf_"
// are present, and returns them. timeout is the maximum time to wait (e.g. 5*time.Minute).
//
// Example:
//   cookies, err := SpawnChromeAndCollectCFCookies("/usr/bin/google-chrome", "", "https://example.com/", 5*time.Minute)
// SpawnChromeAndCollectCFCookies spawns a visible Chrome window and navigates to targetURL.
// Waits for the user to pass Cloudflare challenge and returns all cookies whose names start with "cf_".
// SpawnChromeAndCollectCFCookies opens a visible Chrome browser using chromePath and profileDir,
// navigates to targetURL, waits for the user to defeat Cloudflare, and returns all cookies whose names start with "cf_".
func SpawnChromeAndCollectCFCookies(chromePath, profileDir, targetURL string, timeout time.Duration) ([]SavedCookie, error) {
	cleanupProfile := false
	if profileDir == "" {
		tmp, err := os.MkdirTemp("", "chromedp-profile-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp profile dir: %w", err)
		}
		profileDir = tmp
		cleanupProfile = true
	}

	// allocator options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("headless", false),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	// navigate to target URL
	if err := chromedp.Run(ctx, chromedp.Navigate(targetURL)); err != nil {
		if cleanupProfile {
			_ = os.RemoveAll(profileDir)
		}
		return nil, fmt.Errorf("navigate error: %w", err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if cleanupProfile {
				_ = os.RemoveAll(profileDir)
			}
			return nil, fmt.Errorf("timed out waiting for cf_ cookies: %w", ctx.Err())
		case <-ticker.C:
			var allCookies []*network.Cookie
			// fetch cookies using network.GetCookies().Do(ctx)
			err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				c, err := network.GetCookies().Do(ctx)
				if err != nil {
					return err
				}
				allCookies = c
				return nil
			}))
			if err != nil {
				continue
			}

			// filter cf_ cookies
			var cfCookies []SavedCookie
			for _, c := range allCookies {
				if len(c.Name) >= 3 && c.Name[:3] == "cf_" {
					var exp time.Time
					if c.Expires != 0 {
						exp = time.Unix(int64(c.Expires), 0)
					}
					cfCookies = append(cfCookies, SavedCookie{
						Name:     c.Name,
						Value:    c.Value,
						Domain:   c.Domain,
						Path:     c.Path,
						Expires:  exp,
						HttpOnly: c.HTTPOnly,
						Secure:   c.Secure,
					})
				}
			}

			if len(cfCookies) > 0 {
				cancel() // close Chrome
				time.Sleep(250 * time.Millisecond)
				if cleanupProfile {
					_ = os.RemoveAll(profileDir)
				}
				return cfCookies, nil
			}
		}
	}
}


// DoRequestWithCFCookies makes an HTTP GET to the given targetURL using the provided cookies.
// It returns the *http.Response (caller must close resp.Body) and any error.
// The cookies will be attached to the cookie jar for the target's origin.
func DoRequestWithCFCookies(cookies []SavedCookie, targetURL string) (*http.Response, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("parse target url: %w", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	// convert to net/http cookies
	httpCookies := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		httpCookies = append(httpCookies, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Expires:  c.Expires,
			HttpOnly: c.HttpOnly,
			Secure:   c.Secure,
		})
	}

	jar.SetCookies(parsed, httpCookies)

	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// set a browser-like user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}

	return resp, nil
}

// IsCookieValid returns true if the cookie has not expired yet.
func IsCookieValid(c SavedCookie) bool {
	if c.Expires.IsZero() {
		return true // session cookie
	}
	return time.Now().Before(c.Expires)
}

// AreCookiesValid returns true if at least one cookie is valid
func AreCookiesValid(cookies []SavedCookie) bool {
	for _, c := range cookies {
		if IsCookieValid(c) {
			return true
		}
	}
	return false
}



/*

Need to use a system installed chrome binary, a chrome binary installed via SNAP will NOT work.

wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo dpkg -i google-chrome-stable_current_amd64.deb
sudo apt -f install   # fix dependencies if needed

*/