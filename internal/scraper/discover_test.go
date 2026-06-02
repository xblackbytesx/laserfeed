package scraper

import "testing"

func TestDiscoverFeedsInHTML(t *testing.T) {
	html := `<!doctype html><html><head>
		<link rel="alternate" type="application/rss+xml" title="Main RSS" href="/feed.xml">
		<link rel="alternate" type="application/atom+xml" href="https://other.example/atom">
		<link rel="alternate" type="application/feed+json" href="feed.json">
		<link rel="stylesheet" href="/style.css">
	</head><body></body></html>`

	feeds, err := discoverFeedsInHTML(html, "https://site.example/blog/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feeds) != 3 {
		t.Fatalf("expected 3 feeds, got %d: %#v", len(feeds), feeds)
	}
	// Relative href resolved against the page URL (absolute path).
	if feeds[0].URL != "https://site.example/feed.xml" || feeds[0].Title != "Main RSS" {
		t.Errorf("rss: got %+v", feeds[0])
	}
	// Absolute href preserved.
	if feeds[1].URL != "https://other.example/atom" {
		t.Errorf("atom: got %q", feeds[1].URL)
	}
	// Relative href resolved against the directory.
	if feeds[2].URL != "https://site.example/blog/feed.json" {
		t.Errorf("json: got %q", feeds[2].URL)
	}
}

func TestDiscoverFeedsIgnoresNonFeedLinks(t *testing.T) {
	// An Atom document's own links (alternate text/html, self atom) must not be
	// mistaken for advertised feeds — this is what lets a real feed URL pass
	// through Create unchanged.
	xml := `<feed xmlns="http://www.w3.org/2005/Atom">
		<link rel="alternate" type="text/html" href="https://site.example/post"/>
		<link rel="self" type="application/atom+xml" href="https://site.example/atom"/>
	</feed>`
	feeds, err := discoverFeedsInHTML(xml, "https://site.example/atom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feeds) != 0 {
		t.Fatalf("expected no discovered feeds, got %#v", feeds)
	}
}

func TestDiscoverFeedsDedup(t *testing.T) {
	html := `<head>
		<link rel="alternate" type="application/rss+xml" href="https://site.example/feed">
		<link rel="alternate" type="application/rss+xml" href="https://site.example/feed">
	</head>`
	feeds, err := discoverFeedsInHTML(html, "https://site.example/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("expected dedup to 1, got %d", len(feeds))
	}
}
