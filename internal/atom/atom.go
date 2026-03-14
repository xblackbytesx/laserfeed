package atom

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

// xmlSafe strips characters that are illegal in XML 1.0 documents.
// Control characters other than tab (0x09), LF (0x0A), and CR (0x0D) are
// not permitted in XML and will cause xml.Marshal to produce invalid output.
var xmlIllegal = regexp.MustCompile(`[^\x09\x0A\x0D\x20-\x{D7FF}\x{E000}-\x{FFFD}\x{10000}-\x{10FFFF}]`)

func xmlSafe(s string) string {
	return xmlIllegal.ReplaceAllString(s, "")
}

// isAbsoluteURL reports whether s is an absolute http(s) URL.
func isAbsoluteURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	MediaNS string      `xml:"xmlns:media,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    []atomLink  `xml:"link"`
	Author  *atomAuthor `xml:"author,omitempty"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title     string          `xml:"title"`
	ID        string          `xml:"id"`
	Updated   string          `xml:"updated"`
	Published string          `xml:"published"`
	Link      atomLink        `xml:"link"`
	Author    *atomAuthor     `xml:"author,omitempty"`
	Source    *atomSource     `xml:"source,omitempty"`
	Summary   *atomText       `xml:"summary,omitempty"`
	Content   *atomText       `xml:"content,omitempty"`
	Thumbnail *mediaThumbnail `xml:"media:thumbnail,omitempty"`
}

// atomSource identifies the origin feed for an aggregated entry (RFC 4287 §4.2.11).
type atomSource struct {
	ID    string     `xml:"id"`
	Title string     `xml:"title"`
	Link  []atomLink `xml:"link"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomText struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type mediaThumbnail struct {
	URL string `xml:"url,attr"`
}

func GenerateAtom(ch *channel.Channel, articles []*article.Article, feedsByID map[string]*feed.Feed, appBaseURL string) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	selfURL := fmt.Sprintf("%s/channels/%s/feed.rss", appBaseURL, ch.Slug)
	channelURL := fmt.Sprintf("%s/channels/%s", appBaseURL, ch.Slug)

	feed := atomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		MediaNS: "http://search.yahoo.com/mrss/",
		Title:   xmlSafe(ch.Name),
		ID:      channelURL,
		Updated: now,
		Link: []atomLink{
			{Href: selfURL, Rel: "self", Type: "application/atom+xml"},
			{Href: channelURL, Rel: "alternate"},
		},
		Author: &atomAuthor{Name: "LaserFeed"},
	}

	for _, a := range articles {
		// Prefer GUID as the stable Atom entry ID; fall back to URL.
		// Atom requires the id to be a full IRI — bare GUIDs like "article-123"
		// are invalid, so use the URL in that case.
		entryID := a.GUID
		if entryID == "" || !isAbsoluteURL(entryID) {
			entryID = a.URL
		}

		entry := atomEntry{
			Title:     xmlSafe(a.Title),
			ID:        xmlSafe(entryID),
			Updated:   a.PublishedAt.UTC().Format(time.RFC3339),
			Published: a.PublishedAt.UTC().Format(time.RFC3339),
			Link:      atomLink{Href: xmlSafe(a.URL), Rel: "alternate"},
		}
		if a.Author != "" {
			entry.Author = &atomAuthor{Name: xmlSafe(a.Author)}
		}
		if src, ok := feedsByID[a.FeedID]; ok {
			entry.Source = &atomSource{
				ID:    xmlSafe(src.URL),
				Title: xmlSafe(src.Name),
				Link:  []atomLink{{Href: xmlSafe(src.URL), Rel: "self", Type: "application/rss+xml"}},
			}
		}
		if a.Description != "" {
			entry.Summary = &atomText{Type: "html", Value: xmlSafe(a.Description)}
		}
		if a.Content != "" {
			entry.Content = &atomText{Type: "html", Value: xmlSafe(a.Content)}
		}
		if a.ThumbnailURL != "" {
			entry.Thumbnail = &mediaThumbnail{URL: xmlSafe(a.ThumbnailURL)}
		}
		feed.Entries = append(feed.Entries, entry)
	}

	out, err := xml.Marshal(feed)
	if err != nil {
		return nil, fmt.Errorf("marshal atom: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}
