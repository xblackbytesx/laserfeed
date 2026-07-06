package scraper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// jsRenderTimeout bounds a single headless-browser render. JS-heavy pages need
// more than the plain-fetch deadline: navigation, script execution, and the
// settle delay all happen inside this window.
const jsRenderTimeout = 30 * time.Second

// jsSettleDelay is how long to wait after the DOM is ready for client-side
// rendering (fetch calls, hydration) to fill in the content.
const jsSettleDelay = 2 * time.Second

// ErrJSRenderNotConfigured is returned when a feed has JS rendering enabled but
// no browser endpoint is configured.
var ErrJSRenderNotConfigured = errors.New("JS rendering is enabled for this feed but JS_RENDER_WS_URL is not configured")

// checkPublicHost resolves a URL's host and applies the same private-address
// policy as the HTTP client's dialer. The headless browser fetches URLs with
// its own network stack, so this pre-flight check is the only SSRF guard on
// the initial render URL. It cannot cover redirects or subresource loads made
// inside the browser — run the browser container on an isolated network as
// documented in the README.
func checkPublicHost(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse render url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("render url must be http(s), got %q", u.Scheme)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", u.Hostname())
	if err != nil {
		return fmt.Errorf("resolve render host: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("%w: %s resolves to %s", ErrPrivateAddress, u.Hostname(), ip)
		}
	}
	return nil
}

// renderPage loads articleURL in a remote headless browser (CDP endpoint from
// JS_RENDER_WS_URL), waits for client-side rendering to settle, and returns the
// resulting DOM as HTML. Each call opens a fresh tab that is closed on return,
// so renders don't leak state (cookies aside — those are set per-URL) between
// feeds. Callers hold a global fetch slot, which also bounds concurrent tabs.
func (s *Scraper) renderPage(ctx context.Context, articleURL, userAgent, cookies string) (string, error) {
	if s.jsRenderWSURL == "" {
		return "", ErrJSRenderNotConfigured
	}
	if err := checkPublicHost(ctx, articleURL); err != nil {
		return "", err
	}

	rctx, cancel := context.WithTimeout(ctx, jsRenderTimeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(rctx, s.jsRenderWSURL)
	defer allocCancel()
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	actions := []chromedp.Action{network.Enable()}
	if userAgent != "" {
		actions = append(actions, emulation.SetUserAgentOverride(userAgent))
	}
	if cookies != "" {
		parsed, err := http.ParseCookie(cookies)
		if err != nil {
			return "", fmt.Errorf("parse cookie header: %w", err)
		}
		for _, ck := range parsed {
			actions = append(actions, network.SetCookie(ck.Name, ck.Value).WithURL(articleURL))
		}
	}

	var html string
	actions = append(actions,
		chromedp.Navigate(articleURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(jsSettleDelay),
		chromedp.OuterHTML("html", &html),
	)
	if err := chromedp.Run(tabCtx, actions...); err != nil {
		return "", fmt.Errorf("js render: %w", err)
	}
	return html, nil
}
