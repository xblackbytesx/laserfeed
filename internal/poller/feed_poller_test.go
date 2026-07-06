package poller

import (
	"testing"

	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

func TestRedactURLUserinfo(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"http://example.com/feed", "http://example.com/feed"},
		{"https://user:pass@example.com/feed", "https://example.com/feed"},
		{"https://user@example.com/feed", "https://example.com/feed"},
		{"not a url", "not a url"},
	}
	for _, tt := range tests {
		if got := redactURLUserinfo(tt.in); got != tt.want {
			t.Errorf("redactURLUserinfo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveBuiltinFile(t *testing.T) {
	// Explicit file is returned verbatim.
	if got := resolveBuiltinFile("laserfeed-placeholder-3.svg", "guid-1"); got != "laserfeed-placeholder-3.svg" {
		t.Errorf("explicit file: got %q", got)
	}

	// __rotate__ is deterministic per guid and always a known builtin.
	first := resolveBuiltinFile("__rotate__", "article-42")
	second := resolveBuiltinFile("__rotate__", "article-42")
	if first != second {
		t.Errorf("rotate not deterministic: %q != %q", first, second)
	}
	found := false
	for _, s := range builtinSVGs {
		if s == first {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("rotate produced unknown file %q", first)
	}
}

func TestResolveScrapeParams(t *testing.T) {
	ua := "custom-agent"
	sel := "article.body"
	cookies := "a=b; c=d"
	strip := "  .ad\n\n.promo  \n"
	pageStrip := "header\nfooter"
	f := &feed.Feed{
		UserAgent:                &ua,
		ScrapeMethod:             feed.ScrapeMethodSelector,
		ScrapeSelector:           &sel,
		ScrapeSelectorType:       feed.SelectorTypeXPath,
		ScrapeCookies:            &cookies,
		ScrapeStripSelectors:     &strip,
		ScrapePageStripSelectors: &pageStrip,
		ScrapeRenderJS:           true,
	}

	sp := resolveScrapeParams(f, "global-agent")
	if sp.UserAgent != ua {
		t.Errorf("UserAgent: got %q, want %q", sp.UserAgent, ua)
	}
	if sp.Method != "selector" || sp.SelectorType != "xpath" {
		t.Errorf("method/type: got %q/%q", sp.Method, sp.SelectorType)
	}
	if sp.Cookies != cookies {
		t.Errorf("cookies: got %q", sp.Cookies)
	}
	// Strip selectors are split per line and trimmed, blanks dropped.
	if len(sp.StripSelectors) != 2 || sp.StripSelectors[0] != ".ad" || sp.StripSelectors[1] != ".promo" {
		t.Errorf("StripSelectors: got %#v", sp.StripSelectors)
	}
	if len(sp.PageStripSelectors) != 2 {
		t.Errorf("PageStripSelectors: got %#v", sp.PageStripSelectors)
	}
	if !sp.RenderJS {
		t.Error("RenderJS: expected true")
	}
}

func TestResolveScrapeParamsGlobalUAFallback(t *testing.T) {
	f := &feed.Feed{ScrapeMethod: feed.ScrapeMethodReadability}
	sp := resolveScrapeParams(f, "global-agent")
	if sp.UserAgent != "global-agent" {
		t.Errorf("expected global UA fallback, got %q", sp.UserAgent)
	}
	if sp.RenderJS {
		t.Error("RenderJS: expected false by default")
	}
}
