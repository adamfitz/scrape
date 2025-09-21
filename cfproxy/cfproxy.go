package cfproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/elazarl/goproxy"
)

// Config holds settings for the proxy
type Config struct {
	Port         int
	Debug        bool
	ChromePath   string
	SolveTimeout time.Duration
	CacheTimeout time.Duration
}

// ClearanceCache stores solved Cloudflare tokens
type ClearanceCache struct {
	mu        sync.RWMutex
	tokens    map[string]*ClearanceToken
	userAgent string
}

type ClearanceToken struct {
	Cookie    string
	UserAgent string
	Timestamp time.Time
	Domain    string
}

// ProxyServer wraps the goproxy server with Cloudflare solving capability
type ProxyServer struct {
	config Config
	server *http.Server
	cache  *ClearanceCache
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewProxyServer creates a new Cloudflare-bypassing proxy server
func NewProxyServer(port int) *ProxyServer {
	if port == 0 {
		port = 23181
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ProxyServer{
		config: Config{
			Port:         port,
			Debug:        true,
			SolveTimeout: 60 * time.Second, // Increased timeout
			CacheTimeout: 30 * time.Minute,
		},
		cache: &ClearanceCache{
			tokens:    make(map[string]*ClearanceToken),
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start launches the proxy server
func (p *ProxyServer) Start() error {
	addr := fmt.Sprintf(":%d", p.config.Port)

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = p.config.Debug

	// Handle CONNECT requests for HTTPS with SSL bypass
	proxy.OnRequest(goproxy.ReqHostMatches(regexp.MustCompile(".*"))).HandleConnect(goproxy.AlwaysMitm)

	// Main request handler with Cloudflare bypass
	proxy.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if p.config.Debug {
			log.Printf("[REQ] %s %s", r.Method, r.URL.String())
		}

		// Skip non-target requests
		if !p.shouldBypass(r.URL.Host) {
			return r, nil
		}

		// Try to get cached clearance
		domain := p.extractDomain(r.URL.Host)
		token := p.cache.getToken(domain)

		// Apply clearance if we have it
		if token != nil && !p.isTokenExpired(token) {
			r.Header.Set("User-Agent", token.UserAgent)
			r.Header.Set("Cookie", token.Cookie)
			if p.config.Debug {
				log.Printf("[CACHE] Using cached clearance for %s", domain)
			}
			return r, nil
		}

		// If no valid token, solve challenge
		if p.config.Debug {
			log.Printf("[SOLVE] Solving Cloudflare challenge for %s", domain)
		}

		newToken, err := p.solveCloudflareChallengeChrome(r.URL.String())
		if err != nil {
			log.Printf("[ERROR] Failed to solve challenge for %s: %v", domain, err)
			return r, nil // Continue without clearance
		}

		// Cache the new token
		p.cache.setToken(domain, newToken)

		// Apply new clearance
		r.Header.Set("User-Agent", newToken.UserAgent)
		r.Header.Set("Cookie", newToken.Cookie)

		return r, nil
	})

	// Response handler to detect Cloudflare challenges
	proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp != nil {
			if p.config.Debug {
				log.Printf("[RESP] %d %s", resp.StatusCode, resp.Request.URL.Host)
			}

			// If we get a 403/503 with Cloudflare, invalidate cache
			if (resp.StatusCode == 403 || resp.StatusCode == 503) && p.isCloudflarePage(resp) {
				domain := p.extractDomain(resp.Request.URL.Host)
				p.cache.invalidateToken(domain)
				if p.config.Debug {
					log.Printf("[CACHE] Invalidated token for %s due to challenge", domain)
				}
			}
		}
		return resp
	})

	p.server = &http.Server{
		Addr:    addr,
		Handler: proxy,
	}

	// Start server in goroutine
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Printf("[START] Cloudflare bypass proxy listening on %s", addr)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ERROR] Proxy server: %v", err)
		}
	}()

	// Handle shutdown signals
	go p.handleShutdown()

	return nil
}

// Stop gracefully shuts down the proxy
func (p *ProxyServer) Stop() error {
	if p.server == nil {
		return nil
	}

	log.Printf("[STOP] Shutting down proxy")
	p.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.server.Shutdown(ctx)
	p.wg.Wait()
	return err
}

// solveCloudflareChallengeChrome uses Chrome to solve Cloudflare challenges
func (p *ProxyServer) solveCloudflareChallengeChrome(targetURL string) (*ClearanceToken, error) {
	// Detect if we're in a headless environment
	isHeadless := os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
	if !isHeadless {
		// Check if we can actually use the display
		if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "" {
			isHeadless = true
		}
	}

	// Set up Chrome context with conditional headless mode
	opts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", isHeadless),
		chromedp.Flag("disable-gpu", isHeadless),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("exclude-switches", "enable-automation"),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.Flag("ignore-ssl-errors", true),
		chromedp.Flag("ignore-certificate-errors-spki-list", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("allow-running-insecure-content", true),
		chromedp.Flag("disable-features", "VizDisplayCompositor"),
		chromedp.WindowSize(1920, 1080),
		chromedp.UserAgent(p.cache.userAgent),
	}

	// Additional flags for headless environments
	if isHeadless {
		opts = append(opts,
			chromedp.Flag("virtual-time-budget", "25000"), // 25 seconds virtual time
			chromedp.Flag("run-all-compositor-stages-before-draw", true),
			chromedp.Flag("disable-background-timer-throttling", true),
			chromedp.Flag("disable-renderer-backgrounding", true),
			chromedp.Flag("disable-backgrounding-occluded-windows", true),
		)
		log.Printf("[INFO] Running in headless mode (detected server environment)")
	} else {
		log.Printf("[INFO] Running in non-headless mode")
	}

	if p.config.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(p.config.ChromePath))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(p.ctx, opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Increase timeout for headless environments
	timeout := p.config.SolveTimeout
	if isHeadless {
		timeout = 90 * time.Second // Much longer timeout for headless
	}
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	var cookies []*network.Cookie
	var pageTitle string
	var pageContent string

	log.Printf("[SOLVE] Attempting to solve challenge for %s (headless: %v)", targetURL, isHeadless)

	// Navigate and wait for challenge to be solved with more sophisticated detection
	err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.Sleep(5*time.Second), // Longer initial wait for headless
		chromedp.WaitVisible("body", chromedp.ByQuery),
		// Enhanced challenge solving logic
		chromedp.ActionFunc(func(ctx context.Context) error {
			maxWait := 20
			if isHeadless {
				maxWait = 30 // Longer wait for headless
			}

			for i := 0; i < maxWait; i++ {
				time.Sleep(1 * time.Second)

				// Get page content to check for challenge indicators
				chromedp.InnerHTML("html", &pageContent, chromedp.ByQuery).Do(ctx)
				chromedp.Title(&pageTitle).Do(ctx)

				// Check if we're past the challenge
				contentLower := strings.ToLower(pageContent)
				titleLower := strings.ToLower(pageTitle)

				// Indicators that we might be past the challenge
				pastChallenge := !strings.Contains(titleLower, "checking") &&
					!strings.Contains(titleLower, "cloudflare") &&
					!strings.Contains(contentLower, "checking your browser") &&
					!strings.Contains(contentLower, "ddos protection") &&
					(strings.Contains(contentLower, "manga") || strings.Contains(contentLower, "chapter"))

				if pastChallenge {
					log.Printf("[SOLVE] Challenge appears solved, title: %s", pageTitle)
					break
				}

				// Check for cf_clearance cookie
				cookies, _ = network.GetCookies().Do(ctx)
				for _, cookie := range cookies {
					if cookie.Name == "cf_clearance" && len(cookie.Value) > 10 {
						log.Printf("[SOLVE] Found cf_clearance cookie during wait")
						return nil
					}
				}

				if i%5 == 0 {
					log.Printf("[SOLVE] Still waiting for challenge... (%d/%d) Title: %s", i+1, maxWait, pageTitle)
				}
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("chrome navigation failed: %v", err)
	}

	// Extract cf_clearance cookie
	var cfClearance string
	for _, cookie := range cookies {
		if cookie.Name == "cf_clearance" && len(cookie.Value) > 10 {
			cfClearance = cookie.Value
			break
		}
	}

	// Log all cookies for debugging
	log.Printf("[DEBUG] Found %d cookies:", len(cookies))
	for _, cookie := range cookies {
		log.Printf("[DEBUG] Cookie: %s=%s", cookie.Name, cookie.Value[:min(len(cookie.Value), 20)]+"...")
	}

	if cfClearance == "" {
		// Try alternative approach - sometimes cookies are set but not immediately visible
		time.Sleep(2 * time.Second)
		cookies, _ = network.GetCookies().Do(ctx)
		for _, cookie := range cookies {
			if cookie.Name == "cf_clearance" && len(cookie.Value) > 10 {
				cfClearance = cookie.Value
				break
			}
		}
	}

	if cfClearance == "" {
		return nil, fmt.Errorf("no cf_clearance cookie found after %d seconds (headless: %v)", timeout/time.Second, isHeadless)
	}

	// Build cookie string
	var cookieParts []string
	for _, cookie := range cookies {
		cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	cookieString := strings.Join(cookieParts, "; ")

	u, _ := url.Parse(targetURL)
	token := &ClearanceToken{
		Cookie:    cookieString,
		UserAgent: p.cache.userAgent,
		Timestamp: time.Now(),
		Domain:    p.extractDomain(u.Host),
	}

	log.Printf("[SOLVE] Successfully solved challenge for %s (headless: %v)", token.Domain, isHeadless)
	return token, nil
}

// Helper methods for cache management
func (c *ClearanceCache) getToken(domain string) *ClearanceToken {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tokens[domain]
}

func (c *ClearanceCache) setToken(domain string, token *ClearanceToken) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[domain] = token
}

func (c *ClearanceCache) invalidateToken(domain string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, domain)
}

// Helper methods
func (p *ProxyServer) shouldBypass(host string) bool {
	// Add logic to determine which hosts need Cloudflare bypass
	// For now, assume kunmanga needs it
	return strings.Contains(host, "kunmanga") || strings.Contains(host, "kunmanga.com")
}

func (p *ProxyServer) extractDomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return host
}

func (p *ProxyServer) isTokenExpired(token *ClearanceToken) bool {
	return time.Since(token.Timestamp) > p.config.CacheTimeout
}

func (p *ProxyServer) isCloudflarePage(resp *http.Response) bool {
	// Simple check for Cloudflare response
	server := resp.Header.Get("Server")
	cfRay := resp.Header.Get("CF-RAY")
	return strings.Contains(server, "cloudflare") || cfRay != ""
}

func (p *ProxyServer) handleShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	_ = p.Stop()
}

// Client helper function with SSL bypass and extended timeout
func NewProxiedClient(port int) *http.Client {
	if port == 0 {
		port = 23181
	}

	proxyStr := fmt.Sprintf("http://127.0.0.1:%d", port)
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		log.Fatalf("Invalid proxy URL: %v", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second, // Increased dial timeout
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,  // Increased TLS timeout
		ResponseHeaderTimeout: 120 * time.Second, // Much longer response timeout
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Ignore SSL certificate errors
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   180 * time.Second, // 3 minute total timeout
	}
}

// Test function
func TestFetch(port int, target string) error {
	client := NewProxiedClient(port)

	resp, err := client.Get(target)
	if err != nil {
		return fmt.Errorf("fetch failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body failed: %v", err)
	}

	log.Printf("[TEST] %s -> %d (%d bytes)", target, resp.StatusCode, len(body))

	// Check if we got past Cloudflare
	if strings.Contains(string(body), "Checking your browser") {
		return fmt.Errorf("still blocked by Cloudflare")
	}

	return nil
}
