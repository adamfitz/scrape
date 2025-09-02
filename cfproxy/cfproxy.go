package cfproxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"net/url"

	"github.com/elazarl/goproxy"
)

// Config holds settings for the proxy
type Config struct {
	Port int
}

// ProxyServer wraps the goproxy server with control + logging
type ProxyServer struct {
	config Config
	server *http.Server
	wg     sync.WaitGroup
}

// NewProxyServer creates a new Cloudflare-bypassing proxy server
func NewProxyServer(port int) *ProxyServer {
	if port == 0 {
		port = 23181 // default
	}
	return &ProxyServer{
		config: Config{Port: port},
	}
}

// Start launches the proxy
func (p *ProxyServer) Start() error {
	addr := fmt.Sprintf(":%d", p.config.Port)

	// Set up proxy
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	// Middleware: add Cloudflare bypass headers/cookies
	proxy.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		log.Printf("[REQ] %s %s", r.Method, r.URL.String())
		r.Header.Set("User-Agent", getBypassUserAgent())
		if cookie := getClearanceCookie(); cookie != "" {
			r.AddCookie(&http.Cookie{Name: "cf_clearance", Value: cookie})
		}
		return r, nil
	})

	proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp != nil {
			log.Printf("[RESP] %d %s", resp.StatusCode, resp.Request.URL.Host)
		}
		return resp
	})

	// Server wrapper
	p.server = &http.Server{
		Addr:    addr,
		Handler: proxy,
	}

	// Start in goroutine
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Printf("[START] Proxy listening on %s", addr)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ERROR] Proxy server: %v", err)
		}
	}()

	// Capture signals for clean shutdown
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		_ = p.Stop()
	}()

	return nil
}

// Stop gracefully shuts down proxy
func (p *ProxyServer) Stop() error {
	if p.server == nil {
		return nil
	}
	log.Printf("[STOP] Shutting down proxy")
	err := p.server.Close()
	p.wg.Wait()
	return err
}

// ----------------------------------------------------------------------
// Cloudflare Bypass Helpers
// (these can be swapped for more advanced logic later)
// ----------------------------------------------------------------------

func getBypassUserAgent() string {
	// Pretend to be a modern Chrome
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) " +
		"Chrome/115.0 Safari/537.36"
}

func getClearanceCookie() string {
	// TODO: implement real logic to grab/refresh cf_clearance
	//       For now, just read from env
	return os.Getenv("CF_CLEARANCE")
}

// ----------------------------------------------------------------------
// Example helper: client using this proxy
// ----------------------------------------------------------------------

func NewProxiedClient(port int) *http.Client {
	if port == 0 {
		port = 23181
	}
	proxyStr := fmt.Sprintf("http://127.0.0.1:%d", port)
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		log.Fatalf("Invalid proxy URL: %v", err)
	}
	tr := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// Quick test
func TestFetch(port int, target string) {
	client := NewProxiedClient(port)
	resp, err := client.Get(target)
	if err != nil {
		log.Printf("[FETCH ERROR] %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[FETCH] %s -> %d (%d bytes)", target, resp.StatusCode, len(body))
}
