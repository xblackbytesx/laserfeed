package handler

import (
	"testing"

	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

func TestSlugifyOPML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Tech News", "tech-news"},
		{"  Tech & News  ", "tech-news"},
		{"Foo/Bar", "foo-bar"},
		{"---weird---", "weird"},
		{"", ""},
		{"!!!", ""},
		{"already-slug", "already-slug"},
		{"UPPER", "upper"},
		{"emoji 🚀 rocket", "emoji-rocket"},
	}
	for _, tt := range tests {
		if got := slugifyOPML(tt.in); got != tt.want {
			t.Errorf("slugifyOPML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCollectOPMLFeeds_Flat(t *testing.T) {
	outlines := []opmlOutline{
		{Text: "HN", XmlUrl: "https://hnrss.org/frontpage"},
		{Text: "Lobsters", XmlUrl: "https://lobste.rs/rss"},
	}
	got := collectOPMLFeeds(outlines, "")
	if len(got) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(got))
	}
	if got[0].url != "https://hnrss.org/frontpage" || got[0].folderName != "" {
		t.Errorf("unexpected first feed: %+v", got[0])
	}
}

func TestCollectOPMLFeeds_Folder(t *testing.T) {
	outlines := []opmlOutline{
		{
			Text: "Tech",
			Children: []opmlOutline{
				{Text: "HN", XmlUrl: "https://hnrss.org/frontpage"},
				{Text: "Lobsters", XmlUrl: "https://lobste.rs/rss"},
			},
		},
		{Text: "Standalone", XmlUrl: "https://example.com/feed.rss"},
	}
	got := collectOPMLFeeds(outlines, "")
	if len(got) != 3 {
		t.Fatalf("expected 3 feeds, got %d", len(got))
	}
	if got[0].folderName != "Tech" || got[1].folderName != "Tech" {
		t.Errorf("expected first two feeds in 'Tech' folder; got %+v", got[:2])
	}
	if got[2].folderName != "" {
		t.Errorf("standalone feed should not have a folder; got %q", got[2].folderName)
	}
}

func TestCollectOPMLFeeds_NameFallback(t *testing.T) {
	outlines := []opmlOutline{
		{Title: "Title-only", XmlUrl: "https://a.example/rss"},
		{XmlUrl: "https://b.example/rss"},
	}
	got := collectOPMLFeeds(outlines, "")
	if len(got) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(got))
	}
	if got[0].name != "Title-only" {
		t.Errorf("expected name from Title attr, got %q", got[0].name)
	}
	if got[1].name != "https://b.example/rss" {
		t.Errorf("expected URL fallback as name, got %q", got[1].name)
	}
}

func TestFeedOutline(t *testing.T) {
	f := &feed.Feed{Name: "Example", URL: "https://example.com/feed.rss"}
	got := feedOutline(f)
	if got.Type != "rss" {
		t.Errorf("Type = %q, want \"rss\"", got.Type)
	}
	if got.Text != "Example" || got.Title != "Example" {
		t.Errorf("Text/Title = %q/%q, want both \"Example\"", got.Text, got.Title)
	}
	if got.XmlUrl != "https://example.com/feed.rss" {
		t.Errorf("XmlUrl = %q, want feed URL", got.XmlUrl)
	}
}
