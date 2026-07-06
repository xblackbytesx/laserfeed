package scraper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestScraper builds a Scraper with a plain HTTP client so tests can talk
// to httptest servers on loopback, which the hardened dialer refuses.
func newTestScraper() *Scraper {
	return &Scraper{
		client:   &http.Client{Timeout: 10 * time.Second},
		fetchSem: make(chan struct{}, maxConcurrentFetches),
	}
}

func TestScrapeContentSelectorMode(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
			<nav>site menu</nav>
			<div id="content">
				<h2>Headline</h2>
				<p>Body text with a <a href="/about">relative link</a>.</p>
				<img src="data:image/gif;base64,R0lGOD" data-src="/img/pic.jpg" width="800" height="600">
				<span class="ads">buy stuff</span>
				<script>alert("xss")</script>
			</div>
		</body></html>`))
	}))
	defer srv.Close()

	s := newTestScraper()
	got, err := s.ScrapeContent(context.Background(), srv.URL+"/article", ScrapeOptions{
		UserAgent:      "test-agent",
		Method:         "selector",
		Selector:       "#content",
		SelectorType:   "css",
		Cookies:        "session=abc",
		StripSelectors: []string{".ads"},
	})
	if err != nil {
		t.Fatalf("ScrapeContent: %v", err)
	}

	if gotCookie != "session=abc" {
		t.Errorf("cookie header not sent, got %q", gotCookie)
	}
	if !strings.Contains(got, "Headline") {
		t.Errorf("headline missing:\n%s", got)
	}
	// Relative link resolved against the article URL.
	if !strings.Contains(got, srv.URL+"/about") {
		t.Errorf("relative href not resolved:\n%s", got)
	}
	// Lazy image promoted to src and resolved.
	if !strings.Contains(got, srv.URL+"/img/pic.jpg") {
		t.Errorf("lazy image not promoted/resolved:\n%s", got)
	}
	// Strip selector applied; script sanitized away.
	if strings.Contains(got, "buy stuff") {
		t.Errorf("strip selector not applied:\n%s", got)
	}
	if strings.Contains(got, "script") || strings.Contains(got, "alert") {
		t.Errorf("script survived sanitisation:\n%s", got)
	}
	if strings.Contains(got, "site menu") {
		t.Errorf("content outside selector leaked:\n%s", got)
	}
}

func TestScrapeContentReadabilityMode(t *testing.T) {
	para := strings.Repeat("This is a reasonably long sentence of article body text that readability should score well. ", 5)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Story</title></head><body>
			<nav><a href="/">home</a><a href="/news">news</a></nav>
			<article>
				<h1>Big Story</h1>
				<p>` + para + `</p>
				<p>` + para + `</p>
				<p>` + para + `</p>
			</article>
			<footer>copyright</footer>
			<script>trackEverything()</script>
		</body></html>`))
	}))
	defer srv.Close()

	s := newTestScraper()
	got, err := s.ScrapeContent(context.Background(), srv.URL+"/story", ScrapeOptions{
		UserAgent: "test-agent",
		Method:    "readability",
	})
	if err != nil {
		t.Fatalf("ScrapeContent: %v", err)
	}
	if !strings.Contains(got, "reasonably long sentence") {
		t.Errorf("article body missing:\n%s", got)
	}
	if strings.Contains(got, "trackEverything") {
		t.Errorf("script survived:\n%s", got)
	}
}

func TestScrapeContentBodyTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>"))
		filler := strings.Repeat("a", 64*1024)
		for written := 0; written <= maxBodySize; written += len(filler) {
			if _, err := w.Write([]byte(filler)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	s := newTestScraper()
	_, err := s.ScrapeContent(context.Background(), srv.URL, ScrapeOptions{Method: "readability"})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size-limit error, got %v", err)
	}
}

func TestScrapeContentCharsetDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1252")
		// "café" with é encoded as windows-1252 0xE9.
		_, _ = w.Write([]byte("<html><body><div id=\"c\"><p>caf\xe9</p></div></body></html>"))
	}))
	defer srv.Close()

	s := newTestScraper()
	got, err := s.ScrapeContent(context.Background(), srv.URL, ScrapeOptions{
		Method:   "selector",
		Selector: "#c",
	})
	if err != nil {
		t.Fatalf("ScrapeContent: %v", err)
	}
	if !strings.Contains(got, "café") {
		t.Errorf("windows-1252 content not decoded: %q", got)
	}
}

func TestScrapeContentHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestScraper()
	if _, err := s.ScrapeContent(context.Background(), srv.URL, ScrapeOptions{Method: "readability"}); err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestScrapeContentRenderJSNotConfigured(t *testing.T) {
	s := newTestScraper() // jsRenderWSURL empty
	_, err := s.ScrapeContent(context.Background(), "http://example.com/a", ScrapeOptions{
		Method:   "readability",
		RenderJS: true,
	})
	if !errors.Is(err, ErrJSRenderNotConfigured) {
		t.Errorf("expected ErrJSRenderNotConfigured, got %v", err)
	}
}

func TestCheckPublicHost(t *testing.T) {
	ctx := context.Background()
	if err := checkPublicHost(ctx, "http://127.0.0.1/page"); err == nil {
		t.Error("loopback should be rejected")
	}
	if err := checkPublicHost(ctx, "http://192.168.1.10:8080/x"); err == nil {
		t.Error("RFC1918 address should be rejected")
	}
	if err := checkPublicHost(ctx, "ftp://example.com/x"); err == nil {
		t.Error("non-http scheme should be rejected")
	}
	// Public IP literal needs no DNS and must pass.
	if err := checkPublicHost(ctx, "https://93.184.216.34/x"); err != nil {
		t.Errorf("public IP rejected: %v", err)
	}
}
