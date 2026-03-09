package feedout

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
)

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	MediaNS string      `xml:"xmlns:media,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    []atomLink  `xml:"link"`
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
	Summary   *atomText       `xml:"summary,omitempty"`
	Content   *atomText       `xml:"content,omitempty"`
	Thumbnail *mediaThumbnail `xml:"media:thumbnail,omitempty"`
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

func GenerateAtom(ch *channel.Channel, articles []*article.Article, appBaseURL string) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	selfURL := fmt.Sprintf("%s/channels/%s/feed.rss", appBaseURL, ch.Slug)
	channelURL := fmt.Sprintf("%s/channels/%s", appBaseURL, ch.Slug)

	feed := atomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		MediaNS: "http://search.yahoo.com/mrss/",
		Title:   ch.Name,
		ID:      channelURL,
		Updated: now,
		Link: []atomLink{
			{Href: selfURL, Rel: "self", Type: "application/atom+xml"},
			{Href: channelURL, Rel: "alternate"},
		},
	}

	for _, a := range articles {
		// Prefer GUID as the stable Atom entry ID; fall back to URL.
		entryID := a.GUID
		if entryID == "" {
			entryID = a.URL
		}

		entry := atomEntry{
			Title:     a.Title,
			ID:        entryID,
			Updated:   a.PublishedAt.UTC().Format(time.RFC3339),
			Published: a.PublishedAt.UTC().Format(time.RFC3339),
			Link:      atomLink{Href: a.URL, Rel: "alternate"},
		}
		if a.Author != "" {
			entry.Author = &atomAuthor{Name: a.Author}
		}
		if a.Description != "" {
			entry.Summary = &atomText{Type: "html", Value: a.Description}
		}
		if a.Content != "" {
			entry.Content = &atomText{Type: "html", Value: a.Content}
		}
		if a.ThumbnailURL != "" {
			entry.Thumbnail = &mediaThumbnail{URL: a.ThumbnailURL}
		}
		feed.Entries = append(feed.Entries, entry)
	}

	out, err := xml.Marshal(feed)
	if err != nil {
		return nil, fmt.Errorf("marshal atom: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}
