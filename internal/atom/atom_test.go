package atom

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

func TestGenerateAtom_WellFormedAndContainsExpected(t *testing.T) {
	ch := &channel.Channel{Slug: "tech", Name: "Tech"}
	f := &feed.Feed{ID: "feed-1", Name: "Hacker News", URL: "https://hnrss.org/frontpage"}
	feedsByID := map[string]*feed.Feed{"feed-1": f}

	pub := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	a := &article.Article{
		FeedID:       "feed-1",
		GUID:         "https://example.com/post/1",
		Title:        "Hello & welcome",
		URL:          "https://example.com/post/1",
		Author:       "Alice",
		Description:  "<p>Summary</p>",
		Content:      "<p>Body</p>",
		ThumbnailURL: "https://example.com/img.png",
		PublishedAt:  pub,
	}

	out, err := GenerateAtom(ch, []*article.Article{a}, feedsByID, "https://feeds.example.com")
	if err != nil {
		t.Fatalf("GenerateAtom: %v", err)
	}

	// Must parse as valid XML.
	var sink struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(out, &sink); err != nil {
		t.Fatalf("output is not well-formed XML: %v\n%s", err, out)
	}

	body := string(out)
	wantContains := []string{
		`<?xml version="1.0"`,
		`<title>Tech</title>`,
		`https://feeds.example.com/channels/tech/feed.rss`,
		`<title>Hello &amp; welcome</title>`,
		`<name>Alice</name>`,
		`Body`,
		`media:thumbnail`,
		`<source>`,
	}
	for _, w := range wantContains {
		if !strings.Contains(body, w) {
			t.Errorf("expected output to contain %q\n--- got ---\n%s", w, body)
		}
	}
}

func TestGenerateAtom_StripsIllegalControlChars(t *testing.T) {
	ch := &channel.Channel{Slug: "x", Name: "X"}
	a := &article.Article{
		Title:       "bad\x01title",
		URL:         "https://example.com/1",
		PublishedAt: time.Now().UTC(),
	}
	out, err := GenerateAtom(ch, []*article.Article{a}, nil, "https://feeds.example.com")
	if err != nil {
		t.Fatalf("GenerateAtom: %v", err)
	}
	if strings.ContainsRune(string(out), '\x01') {
		t.Error("output still contains the illegal control character")
	}
	if !strings.Contains(string(out), "<title>badtitle</title>") {
		t.Errorf("expected stripped title, got: %s", out)
	}
}

func TestGenerateAtom_NonAbsoluteGUIDFallsBackToURL(t *testing.T) {
	ch := &channel.Channel{Slug: "x", Name: "X"}
	a := &article.Article{
		GUID:        "post-42", // bare ID — invalid as Atom IRI
		Title:       "t",
		URL:         "https://example.com/post/42",
		PublishedAt: time.Now().UTC(),
	}
	out, err := GenerateAtom(ch, []*article.Article{a}, nil, "https://feeds.example.com")
	if err != nil {
		t.Fatalf("GenerateAtom: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, "<id>https://example.com/post/42</id>") {
		t.Errorf("expected entry id to fall back to URL; got: %s", body)
	}
}
