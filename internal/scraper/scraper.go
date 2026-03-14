package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

const maxBodySize = 5 * 1024 * 1024 // 5MB

// perScrapeTimeout is applied to each individual article fetch independently
// of any outer poll timeout, so one slow page doesn't starve the rest.
const perScrapeTimeout = 15 * time.Second

var bestEffortSelectors = []string{
	"article",
	"[role=main]",
	".article-body",
	".post-content",
	".entry-content",
	"main",
	"body",
}

// readerPolicy is a bluemonday policy that keeps semantic article content
// while stripping scripts, ads, nav bars, inline styles, and other noise.
var readerPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Extend UGC policy with structural/semantic elements found in article bodies.
	p.AllowElements("div", "section", "article", "figure", "figcaption",
		"time", "abbr", "address", "details", "summary", "mark", "small",
		"sub", "sup", "caption", "aside")
	p.AllowAttrs("datetime").OnElements("time")
	p.AllowAttrs("title").OnElements("abbr")
	p.AllowAttrs("open").OnElements("details")
	return p
}()

// SanitizeHTML sanitizes untrusted HTML from RSS feeds using the reader policy,
// stripping scripts, event handlers, and other dangerous content.
func SanitizeHTML(html string) string {
	return readerPolicy.Sanitize(html)
}

type Scraper struct {
	client *http.Client
}

func New() *Scraper {
	return &Scraper{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Scraper) FetchFeed(ctx context.Context, url, userAgent string) (*gofeed.Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("feed server returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	parser := gofeed.NewParser()
	feed, err := parser.ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return feed, nil
}

// ScrapeContent fetches articleURL and extracts reader-view HTML. Each call has its
// own 15-second deadline so a slow page cannot stall an entire poll cycle.
// cookies is an optional raw Cookie header value (e.g. "foo=bar; baz=qux").
func (s *Scraper) ScrapeContent(ctx context.Context, articleURL, userAgent, selector, selectorType, cookies string) (string, error) {
	sctx, cancel := context.WithTimeout(ctx, perScrapeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(sctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("page returned HTTP %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var raw string
	if selector == "" {
		raw, err = bestEffortExtract(string(body))
	} else if selectorType == "xpath" {
		raw, err = extractXPath(string(body), selector)
	} else {
		raw, err = extractCSS(string(body), selector)
	}
	if err != nil {
		return "", err
	}

	return readerPolicy.Sanitize(raw), nil
}

func extractCSS(body, selector string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}
	sel := doc.Find(selector).First()
	html, err := sel.Html()
	if err != nil {
		return "", fmt.Errorf("extract html: %w", err)
	}
	return html, nil
}

func extractXPath(body, selector string) (string, error) {
	doc, err := htmlquery.Parse(strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}
	nodes := htmlquery.Find(doc, selector)
	if len(nodes) == 0 {
		return "", nil
	}
	return htmlquery.OutputHTML(nodes[0], true), nil
}

func bestEffortExtract(body string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}
	for _, sel := range bestEffortSelectors {
		node := doc.Find(sel).First()
		if node.Length() > 0 {
			html, err := node.Html()
			if err == nil && html != "" {
				return html, nil
			}
		}
	}
	return "", nil
}
