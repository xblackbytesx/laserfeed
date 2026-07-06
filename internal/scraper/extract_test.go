package scraper

import (
	"strings"
	"testing"
)

func TestExtractCSS(t *testing.T) {
	page := `<html><body><div id="content"><p>hello</p></div><div id="other">no</div></body></html>`
	got, err := extractCSS(page, "#content")
	if err != nil {
		t.Fatalf("extractCSS: %v", err)
	}
	if !strings.Contains(got, "<p>hello</p>") {
		t.Errorf("expected paragraph in fragment, got %q", got)
	}
	if strings.Contains(got, "no") {
		t.Errorf("unexpected content from other element: %q", got)
	}

	// No match yields empty output (reported as scrape failure by the caller).
	got, err = extractCSS(page, ".missing")
	if err != nil {
		t.Fatalf("extractCSS no-match: %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty fragment for no match, got %q", got)
	}
}

func TestExtractXPath(t *testing.T) {
	page := `<html><body><article class="story"><p>xpath text</p></article></body></html>`
	got, err := extractXPath(page, `//article[@class='story']`)
	if err != nil {
		t.Fatalf("extractXPath: %v", err)
	}
	if !strings.Contains(got, "xpath text") {
		t.Errorf("expected article content, got %q", got)
	}

	got, err = extractXPath(page, `//div[@id='missing']`)
	if err != nil {
		t.Fatalf("extractXPath no-match: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty output for no match, got %q", got)
	}
}

func TestApplyStripSelectors(t *testing.T) {
	fragment := `<div><p>keep</p><span class="ads">drop</span><nav>drop too</nav></div>`
	got, err := applyStripSelectors(fragment, []string{".ads", "nav", ""})
	if err != nil {
		t.Fatalf("applyStripSelectors: %v", err)
	}
	if !strings.Contains(got, "keep") || strings.Contains(got, "drop") {
		t.Errorf("strip failed: %q", got)
	}
}

func TestScopeStripSelectors(t *testing.T) {
	scoped := scopeStripSelectors([]string{".ad", "h1"}, "article.body")
	if scoped[0] != "article.body .ad" || scoped[1] != "article.body h1" {
		t.Errorf("got %#v", scoped)
	}
	// Empty scope returns selectors unchanged.
	same := scopeStripSelectors([]string{".ad"}, "")
	if same[0] != ".ad" {
		t.Errorf("got %#v", same)
	}
}

func TestNormalizeLazyImages(t *testing.T) {
	page := `<html><body>
		<img src="data:image/gif;base64,R0lGOD" data-src="/real.jpg">
		<img data-lazy-src="/lazy.jpg">
		<img data-srcset="/a.jpg 1x, /b.jpg 2x">
		<img src="/normal.jpg">
	</body></html>`
	got := normalizeLazyImages(page)
	for _, want := range []string{`src="/real.jpg"`, `src="/lazy.jpg"`, `srcset="/a.jpg 1x, /b.jpg 2x"`, `src="/normal.jpg"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `src="data:image/gif`) {
		t.Errorf("placeholder src not replaced:\n%s", got)
	}

	// A page without lazy images is returned byte-identical (no reserialization).
	plain := `<html><body><img src="/x.jpg"><p>text</p></body></html>`
	if normalizeLazyImages(plain) != plain {
		t.Error("page without lazy images should be returned unchanged")
	}
}

func TestResolveRelativeURLs(t *testing.T) {
	fragment := `<p><a href="/about">about</a><a href="https://other.example/full">full</a>` +
		`<img src="images/pic.jpg"><img src="//cdn.example.com/c.jpg">` +
		`<img srcset="/a.jpg 480w, b.jpg 800w, javascript:evil() 1x">` +
		`<video poster="/poster.jpg"></video><a href="#frag">frag</a></p>`
	got := resolveRelativeURLs(fragment, "https://site.example/news/story.html")

	wants := []string{
		`href="https://site.example/about"`,
		`href="https://other.example/full"`,
		`src="https://site.example/news/images/pic.jpg"`,
		`src="https://cdn.example.com/c.jpg"`,
		`https://site.example/a.jpg 480w`,
		`https://site.example/news/b.jpg 800w`,
		`poster="https://site.example/poster.jpg"`,
		`href="#frag"`,
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: srcset entry not dropped:\n%s", got)
	}
}

func TestResolveSrcsetDropsNonHTTP(t *testing.T) {
	identity := func(s string) string { return s }
	got := resolveSrcset("https://a.example/x.jpg 1x, data:image/gif;base64,AA 2x", identity)
	if got != "https://a.example/x.jpg 1x" {
		t.Errorf("got %q", got)
	}
}

func TestUnwrapReadability(t *testing.T) {
	html := `<div id="readability-page-1"><div class="site-wrap" id="main">` +
		`<p>content</p><div>   </div></div></div>`
	got := unwrapReadability(html)
	if strings.Contains(got, "readability-page-1") {
		t.Errorf("wrapper not removed: %q", got)
	}
	if strings.Contains(got, "site-wrap") || strings.Contains(got, `id="main"`) {
		t.Errorf("div class/id not stripped: %q", got)
	}
	if !strings.Contains(got, "<p>content</p>") {
		t.Errorf("content lost: %q", got)
	}
	if strings.Contains(got, "<div> ") || strings.Contains(got, "<div></div>") {
		t.Errorf("empty div not removed: %q", got)
	}
}

func TestReadBodyLimited(t *testing.T) {
	small := strings.NewReader("hello")
	if body, err := readBodyLimited(small); err != nil || string(body) != "hello" {
		t.Errorf("small body: %q, %v", body, err)
	}
	big := strings.NewReader(strings.Repeat("a", maxBodySize+1))
	if _, err := readBodyLimited(big); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size error, got %v", err)
	}
	// Exactly at the limit is accepted.
	exact := strings.NewReader(strings.Repeat("a", maxBodySize))
	if body, err := readBodyLimited(exact); err != nil || len(body) != maxBodySize {
		t.Errorf("exact-limit body: len=%d, %v", len(body), err)
	}
}

func TestDecodeHTML(t *testing.T) {
	// "café" in windows-1252: é = 0xE9.
	raw := []byte("<html><body>caf\xe9</body></html>")
	got := decodeHTML(raw, "text/html; charset=windows-1252")
	if !strings.Contains(got, "café") {
		t.Errorf("windows-1252 not decoded: %q", got)
	}
	// Plain UTF-8 passes through.
	utf8 := []byte("<html><body>café</body></html>")
	if got := decodeHTML(utf8, "text/html; charset=utf-8"); !strings.Contains(got, "café") {
		t.Errorf("utf-8 mangled: %q", got)
	}
}

func TestSrcsetBest(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/a.jpg 480w, /b.jpg 1200w, /c.jpg 800w", "/b.jpg"},
		{"/x.jpg 1x, /y.jpg 2x", "/y.jpg"},
		{"/only.jpg", "/only.jpg"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := srcsetBest(tt.in); got != tt.want {
			t.Errorf("srcsetBest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
