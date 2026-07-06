package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html/charset"
)

// ErrPrivateAddress is returned by the dialer when a feed/article URL resolves
// to a non-routable destination (loopback, private, link-local, etc.). This
// prevents a malicious feed from probing internal services on the host network.
var ErrPrivateAddress = errors.New("scraper: refusing to connect to private address")

// isPrivateIP reports whether ip is in any range we refuse to dial.
// We block: loopback, link-local, multicast, unspecified, private (RFC1918,
// RFC4193, IPv4 carrier-grade NAT 100.64.0.0/10), and the AWS/GCP/Azure
// metadata IPv4 169.254.169.254 (covered by link-local).
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() ||
		ip.IsPrivate() {
		return true
	}
	// IPv4 carrier-grade NAT — not covered by net.IP.IsPrivate.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// safeDialContext wraps a net.Dialer so connections to private addresses are
// rejected after DNS resolution but before the TCP connect.
func safeDialContext(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := d.Resolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("%w: %s resolves to %s", ErrPrivateAddress, host, ip)
			}
		}
		// Dial the checked IPs directly (not the hostname) to avoid TOCTOU
		// between our check and a second resolution, falling back through the
		// list so a host whose first record is unreachable — typically an AAAA
		// record on an IPv4-only network — still connects.
		var firstErr error
		for _, ip := range ips {
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		return nil, firstErr
	}
}

const maxBodySize = 5 * 1024 * 1024 // 5MB

// maxRedirects caps how many redirects a single fetch will follow. Each hop is
// still re-checked by safeDialContext, so this mainly bounds redirect-chain
// abuse rather than SSRF (which the dialer already prevents).
const maxRedirects = 5

// maxConcurrentFetches bounds how many outbound fetches — feed polls, page
// discovery, and article scrapes — run at once across all goroutines. It
// prevents a thundering herd (e.g. every feed's timer firing at once after a
// restart, or several feeds scraping simultaneously at 4 pages each) from
// opening an unbounded number of simultaneous connections.
const maxConcurrentFetches = 8

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
	// srcset/sizes for responsive images. bluemonday does not URL-validate
	// srcset (only href/src), so resolveSrcset scheme-filters entries before
	// content reaches the sanitizer.
	p.AllowAttrs("srcset", "sizes").OnElements("img")
	return p
}()

// SanitizeHTML sanitizes untrusted HTML from RSS feeds using the reader policy,
// stripping scripts, event handlers, and other dangerous content.
func SanitizeHTML(html string) string {
	return readerPolicy.Sanitize(html)
}

type Scraper struct {
	client        *http.Client
	fetchSem      chan struct{} // bounds concurrent feed/page fetches
	jsRenderWSURL string        // CDP endpoint for JS rendering; empty = not configured
}

// New builds the hardened scraper. jsRenderWSURL is the optional DevTools
// websocket endpoint (e.g. "ws://laserfeed-chrome:9222/") used for feeds with
// JS rendering enabled; pass "" to leave the feature unconfigured.
func New(jsRenderWSURL string) *Scraper {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver:  net.DefaultResolver,
	}
	transport := &http.Transport{
		DialContext:           safeDialContext(dialer),
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   5,
		MaxConnsPerHost:       10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Scraper{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return nil
			},
		},
		fetchSem:      make(chan struct{}, maxConcurrentFetches),
		jsRenderWSURL: jsRenderWSURL,
	}
}

// acquireFetch blocks until a global fetch slot is free or ctx is cancelled.
// The returned release func must be called when the fetch completes.
func (s *Scraper) acquireFetch(ctx context.Context) (release func(), err error) {
	select {
	case s.fetchSem <- struct{}{}:
		return func() { <-s.fetchSem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readBodyLimited reads at most maxBodySize bytes and errors when the body is
// larger, rather than silently truncating — a truncated HTML page or feed
// extracts garbage with no diagnostic.
func readBodyLimited(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) > maxBodySize {
		return nil, fmt.Errorf("body exceeds %d MB limit", maxBodySize>>20)
	}
	return body, nil
}

// decodeHTML converts a raw HTML body to UTF-8 using the Content-Type header,
// BOM, and <meta charset> sniffing. Best effort: on any failure the body is
// returned as-is (most of the web is UTF-8 already).
func decodeHTML(body []byte, contentType string) string {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return string(body)
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return string(body)
	}
	return string(decoded)
}

// doFetchPage GETs a URL through the hardened client and returns the
// size-limited body plus its Content-Type. Callers must hold a fetch slot.
func (s *Scraper) doFetchPage(ctx context.Context, pageURL, userAgent, cookies string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("page returned HTTP %s", resp.Status)
	}

	body, err := readBodyLimited(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// fetchPage is doFetchPage bounded by the global fetch semaphore.
func (s *Scraper) fetchPage(ctx context.Context, pageURL, userAgent, cookies string) ([]byte, string, error) {
	release, err := s.acquireFetch(ctx)
	if err != nil {
		return nil, "", err
	}
	defer release()
	return s.doFetchPage(ctx, pageURL, userAgent, cookies)
}

// FeedResult is the outcome of a (possibly conditional) feed fetch.
type FeedResult struct {
	Feed         *gofeed.Feed // nil when NotModified
	NotModified  bool         // server returned 304
	ETag         string       // ETag to store for the next conditional request
	LastModified string       // Last-Modified to store for the next conditional request
}

// FetchFeed retrieves and parses a feed. When prevETag/prevLastModified are
// non-empty they are sent as If-None-Match/If-Modified-Since; a 304 response
// yields NotModified (and a nil Feed) so the caller can skip re-parsing.
func (s *Scraper) FetchFeed(ctx context.Context, url, userAgent, prevETag, prevLastModified string) (*FeedResult, error) {
	release, err := s.acquireFetch(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if prevETag != "" {
		req.Header.Set("If-None-Match", prevETag)
	}
	if prevLastModified != "" {
		req.Header.Set("If-Modified-Since", prevLastModified)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		// Carry the previous validators forward; the server confirmed they're current.
		return &FeedResult{NotModified: true, ETag: prevETag, LastModified: prevLastModified}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("feed server returned %s", resp.Status)
	}

	body, err := readBodyLimited(resp.Body)
	if err != nil {
		return nil, err
	}

	parser := gofeed.NewParser()
	feed, err := parser.ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return &FeedResult{
		Feed:         feed,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// DiscoveredFeed is a feed link advertised by an HTML page.
type DiscoveredFeed struct {
	URL   string
	Title string
}

// DiscoverFeeds fetches an HTML page and returns any RSS/Atom/JSON feeds it
// advertises via <link rel="alternate" type="application/(rss|atom|feed)+...">.
// Relative hrefs are resolved against pageURL. Uses the same hardened client.
func (s *Scraper) DiscoverFeeds(ctx context.Context, pageURL, userAgent string) ([]DiscoveredFeed, error) {
	body, contentType, err := s.fetchPage(ctx, pageURL, userAgent, "")
	if err != nil {
		return nil, err
	}
	return discoverFeedsInHTML(decodeHTML(body, contentType), pageURL)
}

// discoverFeedsInHTML is the pure parsing half of DiscoverFeeds (unit-testable).
func discoverFeedsInHTML(htmlBody, pageURL string) ([]DiscoveredFeed, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("parse page url: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	var feeds []DiscoveredFeed
	seen := map[string]bool{}
	doc.Find(`link[rel~="alternate"]`).Each(func(_ int, sel *goquery.Selection) {
		switch typ, _ := sel.Attr("type"); typ {
		case "application/rss+xml", "application/atom+xml", "application/feed+json":
		default:
			return
		}
		href, ok := sel.Attr("href")
		if !ok || strings.TrimSpace(href) == "" {
			return
		}
		ref, err := url.Parse(strings.TrimSpace(href))
		if err != nil {
			return
		}
		abs := base.ResolveReference(ref).String()
		if seen[abs] {
			return
		}
		seen[abs] = true
		title, _ := sel.Attr("title")
		feeds = append(feeds, DiscoveredFeed{URL: abs, Title: strings.TrimSpace(title)})
	})
	return feeds, nil
}

// ScrapeOptions carries the per-feed configuration for a content scrape.
type ScrapeOptions struct {
	UserAgent          string
	Method             string // "readability" (default) or "selector"
	Selector           string
	SelectorType       string // "css" (default) or "xpath"
	Cookies            string // raw Cookie header value
	StripSelectors     []string
	PageStripSelectors []string
	RenderJS           bool // render the page in a headless browser before extraction
}

// ScrapeContent fetches articleURL and extracts reader-view HTML. Each call has
// its own network deadline so a slow page cannot stall an entire poll cycle.
//
// opts.Method controls the extraction pipeline:
//
//	"readability" — strip selectors on raw page (classes intact) → readability → unwrap → sanitize
//	"selector"    — extract via CSS/XPath content selector → strip selectors on fragment → resolve URLs → sanitize
func (s *Scraper) ScrapeContent(ctx context.Context, articleURL string, opts ScrapeOptions) (string, error) {
	// Wait for a global fetch slot against the caller's context, so queueing
	// under load doesn't eat into the per-scrape network deadline. The slot is
	// released as soon as the page is obtained; extraction is local CPU work.
	release, err := s.acquireFetch(ctx)
	if err != nil {
		return "", err
	}
	var page string
	if opts.RenderJS {
		page, err = s.renderPage(ctx, articleURL, opts.UserAgent, opts.Cookies)
	} else {
		var body []byte
		var contentType string
		sctx, cancel := context.WithTimeout(ctx, perScrapeTimeout)
		body, contentType, err = s.doFetchPage(sctx, articleURL, opts.UserAgent, opts.Cookies)
		cancel()
		if err == nil {
			page = decodeHTML(body, contentType)
		}
	}
	release()
	if err != nil {
		return "", err
	}

	// Promote lazy-loading image attributes (data-src etc.) to real src/srcset
	// before extraction, so images survive both extraction and sanitisation.
	page = normalizeLazyImages(page)

	var raw string
	if opts.Method == "selector" {
		// Selector mode: extract content fragment, then apply both strip lists on the fragment.
		if opts.SelectorType == "xpath" {
			raw, err = extractXPath(page, opts.Selector)
		} else {
			raw, err = extractCSS(page, opts.Selector)
		}
		if err != nil {
			return "", err
		}
		combined := make([]string, 0, len(opts.PageStripSelectors)+len(opts.StripSelectors))
		combined = append(combined, opts.PageStripSelectors...)
		combined = append(combined, opts.StripSelectors...)
		if len(combined) > 0 {
			if stripped, err := applyStripSelectors(raw, combined); err == nil {
				raw = stripped
			}
		}
		// Readability resolves relative URLs itself; selector extraction must do
		// it explicitly or site-relative images/links break inside RSS readers.
		raw = resolveRelativeURLs(raw, articleURL)
	} else {
		// Readability mode: two-pass strip on the raw page (classes still intact),
		// then run readability on the cleaned page.
		// Pass 1: page strip selectors run unscoped against the full page.
		// Pass 2: content strip selectors run scoped by the content selector.
		if len(opts.PageStripSelectors) > 0 {
			if stripped, err := applyStripSelectors(page, opts.PageStripSelectors); err == nil {
				page = stripped
			}
		}
		if len(opts.StripSelectors) > 0 {
			scoped := scopeStripSelectors(opts.StripSelectors, opts.Selector)
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

// lazySrcAttrs are attributes lazy-loading libraries stash the real image URL
// in while src holds a placeholder (or nothing) until JS swaps it in.
var lazySrcAttrs = []string{"data-src", "data-lazy-src", "data-original"}
var lazySrcsetAttrs = []string{"data-srcset", "data-lazy-srcset"}

// normalizeLazyImages promotes lazy-loading attributes to real src/srcset so
// images survive extraction and sanitisation (which strips data-* attributes).
// Returns the page unchanged when no lazy images are found.
func normalizeLazyImages(page string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		return page
	}
	changed := false
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		// A data: src is a placeholder (blank GIF / blurred inline preview).
		if src, _ := s.Attr("src"); src == "" || strings.HasPrefix(src, "data:") {
			for _, attr := range lazySrcAttrs {
				if v, ok := s.Attr(attr); ok && strings.TrimSpace(v) != "" {
					s.SetAttr("src", strings.TrimSpace(v))
					changed = true
					break
				}
			}
		}
		if srcset, _ := s.Attr("srcset"); srcset == "" {
			for _, attr := range lazySrcsetAttrs {
				if v, ok := s.Attr(attr); ok && strings.TrimSpace(v) != "" {
					s.SetAttr("srcset", strings.TrimSpace(v))
					changed = true
					break
				}
			}
		}
	})
	if !changed {
		return page
	}
	out, err := doc.Html()
	if err != nil {
		return page
	}
	return out
}

// resolveRelativeURLs rewrites relative src/href/poster/srcset attributes in an
// HTML fragment to absolute URLs against the article URL. srcset entries with a
// scheme other than http(s) are dropped because the sanitizer does not
// URL-validate srcset.
func resolveRelativeURLs(fragment, baseURL string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return fragment
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
	if err != nil {
		return fragment
	}
	resolve := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "#") {
			return raw
		}
		ref, err := url.Parse(raw)
		if err != nil || ref.IsAbs() {
			return raw
		}
		return base.ResolveReference(ref).String()
	}
	for _, attr := range []string{"src", "href", "poster"} {
		doc.Find("[" + attr + "]").Each(func(_ int, s *goquery.Selection) {
			if v, ok := s.Attr(attr); ok {
				s.SetAttr(attr, resolve(v))
			}
		})
	}
	doc.Find("[srcset]").Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("srcset"); ok {
			s.SetAttr("srcset", resolveSrcset(v, resolve))
		}
	})
	out, err := doc.Find("body").Html()
	if err != nil {
		return fragment
	}
	return out
}

// resolveSrcset resolves each URL in a srcset value, preserving width/density
// descriptors and dropping entries that don't resolve to http(s). Splitting on
// "," is imperfect for data: URIs containing commas, but those are dropped
// entries anyway.
func resolveSrcset(srcset string, resolve func(string) string) string {
	parts := strings.Split(srcset, ",")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		fields[0] = resolve(fields[0])
		if !strings.HasPrefix(fields[0], "http://") && !strings.HasPrefix(fields[0], "https://") {
			continue
		}
		kept = append(kept, strings.Join(fields, " "))
	}
	return strings.Join(kept, ", ")
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
	// children are removed. Iteration capped to avoid pathological loops.
	for i := 0; i < 50; i++ {
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
