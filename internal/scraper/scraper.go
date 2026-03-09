package scraper

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"github.com/mmcdole/gofeed"
)

const maxBodySize = 5 * 1024 * 1024 // 5MB

var bestEffortSelectors = []string{
	"article",
	"[role=main]",
	".article-body",
	".post-content",
	".entry-content",
	"main",
	"body",
}

type Scraper struct {
	client *http.Client
}

func New() *Scraper {
	return &Scraper{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchFeed fetches and parses an RSS/Atom feed using a custom user agent.
func (s *Scraper) FetchFeed(url, userAgent string) (*gofeed.Feed, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

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

// ScrapeContent fetches a URL and extracts content using the given selector.
func (s *Scraper) ScrapeContent(articleURL, userAgent, selector, selectorType string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, articleURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch article: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	if selector == "" {
		return bestEffortExtract(string(body))
	}

	switch selectorType {
	case "xpath":
		return extractXPath(string(body), selector)
	default:
		return extractCSS(string(body), selector)
	}
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
