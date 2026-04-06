package scraper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

const maxBodySize = 5 * 1024 * 1024 // 5MB

// perScrapeTimeout is applied to each individual article fetch independently
// of any outer poll timeout, so one slow page doesn't starve the rest.
const perScrapeTimeout = 15 * time.Second

// readerPolicy is a bluemonday policy that keeps semantic article content
// while stripping scripts, ads, nav bars, inline styles, and other noise.
var readerPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Extend UGC policy with structural/semantic elements found in article bodies.
	// Note: article and section are intentionally excluded — they are layout
	// containers that trigger max-width/padding styles in RSS readers. Bluemonday
	// strips the tags but preserves their inner content, effectively unwrapping them.
	p.AllowElements("div", "figure", "figcaption",
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
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Scraper{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
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
//
// The method parameter controls the extraction pipeline:
//
//	"readability" — strip selectors on raw page (classes intact) → readability → unwrap → sanitize
//	"selector"    — extract via CSS/XPath content selector → strip selectors on fragment → sanitize
func (s *Scraper) ScrapeContent(ctx context.Context, articleURL, userAgent, method, selector, selectorType, cookies string, stripSelectors, pageStripSelectors []string) (string, error) {
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
	if method == "selector" {
		// Selector mode: extract content fragment, then apply both strip lists on the fragment.
		if selectorType == "xpath" {
			raw, err = extractXPath(string(body), selector)
		} else {
			raw, err = extractCSS(string(body), selector)
		}
		if err != nil {
			return "", err
		}
		combined := append(pageStripSelectors, stripSelectors...)
		if len(combined) > 0 {
			if stripped, err := applyStripSelectors(raw, combined); err == nil {
				raw = stripped
			}
		}
	} else {
		// Readability mode: two-pass strip on the raw page (classes still intact),
		// then run readability on the cleaned page.
		// Pass 1: page strip selectors run unscoped against the full page.
		// Pass 2: content strip selectors run scoped by the content selector.
		page := string(body)
		if len(pageStripSelectors) > 0 {
			if stripped, err := applyStripSelectors(page, pageStripSelectors); err == nil {
				page = stripped
			}
		}
		if len(stripSelectors) > 0 {
			scoped := scopeStripSelectors(stripSelectors, selector)
			if stripped, err := applyStripSelectors(page, scoped); err == nil {
				page = stripped
			}
		}
		raw, err = readabilityExtract([]byte(page), articleURL)
		if err != nil {
			return "", err
		}
	}

	return readerPolicy.Sanitize(raw), nil
}

// scopeStripSelectors prepends a scope selector to each strip selector so that
// e.g. scope "article.post-body" + strip ".ad-container" becomes
// "article.post-body .ad-container". If scope is empty, selectors are returned as-is.
func scopeStripSelectors(selectors []string, scope string) []string {
	if scope == "" {
		return selectors
	}
	scoped := make([]string, len(selectors))
	for i, sel := range selectors {
		scoped[i] = scope + " " + sel
	}
	return scoped
}

func applyStripSelectors(htmlFragment string, selectors []string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlFragment))
	if err != nil {
		return htmlFragment, err
	}
	for _, sel := range selectors {
		if sel != "" {
			doc.Find(sel).Remove()
		}
	}
	return doc.Find("body").Html()
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

// readabilityExtract uses Mozilla's Readability algorithm (via go-readability)
// to extract the main article content from a full HTML page. The pageURL is
// used to resolve relative URLs in the extracted content.
func readabilityExtract(body []byte, pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("parse article URL: %w", err)
	}
	article, err := readability.FromReader(bytes.NewReader(body), u)
	if err != nil {
		return "", fmt.Errorf("readability extract: %w", err)
	}
	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		return "", fmt.Errorf("readability render: %w", err)
	}
	return unwrapReadability(buf.String()), nil
}

// unwrapReadability cleans up go-readability output for feed embedding:
//   - removes the outer <div id="readability-page-1"> wrapper
//   - strips id/class attributes from divs (site-specific layout identifiers)
//   - removes empty divs (whitespace-only structural noise from sites like AndroidPolice)
func unwrapReadability(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}
	// Unwrap the readability wrapper: replace it with its children.
	doc.Find("div#readability-page-1").Each(func(_ int, s *goquery.Selection) {
		s.ReplaceWithSelection(s.Children())
	})
	// Strip id and class attributes from divs — they are site-specific
	// layout identifiers that have no meaning inside a feed entry.
	doc.Find("div").Each(func(_ int, s *goquery.Selection) {
		s.RemoveAttr("id")
		s.RemoveAttr("class")
	})
	// Remove empty divs (contain only whitespace). Walk bottom-up so nested
	// empty divs collapse correctly — a parent becomes empty once its empty
	// children are removed.
	for {
		removed := false
		doc.Find("div").Each(func(_ int, s *goquery.Selection) {
			if strings.TrimSpace(s.Text()) == "" && s.Find("img, video, audio, iframe, svg").Length() == 0 {
				s.Remove()
				removed = true
			}
		})
		if !removed {
			break
		}
	}
	result, err := doc.Find("body").Html()
	if err != nil {
		return html
	}
	return result
}
