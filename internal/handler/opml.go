package handler

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

// OPML XML structures.

type opmlDoc struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

type opmlHead struct {
	Title       string `xml:"title"`
	DateCreated string `xml:"dateCreated,omitempty"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XmlUrl   string        `xml:"xmlUrl,attr,omitempty"`
	HtmlUrl  string        `xml:"htmlUrl,attr,omitempty"`
	Children []opmlOutline `xml:"outline"`
}

// feedOutline builds an OPML outline element for a single feed.
func feedOutline(f *feed.Feed) opmlOutline {
	return opmlOutline{
		Type:   "rss",
		Text:   f.Name,
		Title:  f.Name,
		XmlUrl: f.URL,
	}
}

// ExportOPML streams feeds (grouped by channel) as a downloadable OPML file.
func (h *SettingsHandler) ExportOPML(c *echo.Context) error {
	ctx := c.Request().Context()

	feeds, err := h.feeds.List(ctx)
	if err != nil {
		slog.Error("opml export: list feeds", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feeds")
	}

	channels, err := h.channels.List(ctx)
	if err != nil {
		slog.Error("opml export: list channels", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channels")
	}

	// Build a set of feed URLs that belong to at least one channel.
	inChannel := make(map[string]bool, len(feeds))
	// Build channel → feed outlines.
	type channelEntry struct {
		ch       *channel.Channel
		outlines []opmlOutline
	}
	channelEntries := make([]channelEntry, 0, len(channels))

	for _, ch := range channels {
		chFeeds, err := h.channels.ListFeeds(ctx, ch.ID)
		if err != nil {
			slog.Error("opml export: list channel feeds", "channel_id", ch.ID, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
		}
		var outlines []opmlOutline
		for _, f := range chFeeds {
			inChannel[f.URL] = true
			outlines = append(outlines, feedOutline(f))
		}
		channelEntries = append(channelEntries, channelEntry{ch: ch, outlines: outlines})
	}

	var body opmlBody

	// Channels as folder outlines with their feeds as children.
	for _, ce := range channelEntries {
		if len(ce.outlines) == 0 {
			continue
		}
		body.Outlines = append(body.Outlines, opmlOutline{
			Text:     ce.ch.Name,
			Children: ce.outlines,
		})
	}

	// Feeds not in any channel as top-level outlines.
	for _, f := range feeds {
		if !inChannel[f.URL] {
			body.Outlines = append(body.Outlines, feedOutline(f))
		}
	}

	doc := opmlDoc{
		Version: "2.0",
		Head: opmlHead{
			Title:       "LaserFeed Subscriptions",
			DateCreated: time.Now().UTC().Format(time.RFC1123Z),
		},
		Body: body,
	}

	filename := fmt.Sprintf("laserfeed-%s.opml", time.Now().UTC().Format("2006-01-02"))
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+filename)
	c.Response().Header().Set("Content-Type", "application/xml; charset=utf-8")

	if _, err := io.WriteString(c.Response(), `<?xml version="1.0" encoding="UTF-8"?>`+"\n"); err != nil {
		slog.Error("opml export: write xml declaration", "err", err)
		return nil
	}
	enc := xml.NewEncoder(c.Response())
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		slog.Error("opml export: encode xml", "err", err)
	}
	return nil
}

// parsedFeed is an intermediate structure used during OPML import.
type parsedFeed struct {
	name       string
	url        string
	folderName string // empty if not inside a folder outline
}

// collectOPMLFeeds walks OPML outlines and collects feed entries.
// One level of folder nesting is supported; deeper nesting is flattened.
func collectOPMLFeeds(outlines []opmlOutline, folderName string) []parsedFeed {
	var result []parsedFeed
	for _, o := range outlines {
		if o.XmlUrl != "" {
			// It's a feed outline.
			name := o.Text
			if name == "" {
				name = o.Title
			}
			if name == "" {
				name = o.XmlUrl
			}
			result = append(result, parsedFeed{
				name:       name,
				url:        o.XmlUrl,
				folderName: folderName,
			})
		} else if len(o.Children) > 0 {
			// It's a folder outline — recurse, passing folder name.
			folder := o.Text
			if folder == "" {
				folder = folderName
			}
			result = append(result, collectOPMLFeeds(o.Children, folder)...)
		}
	}
	return result
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyOPML converts a folder name to a valid channel slug.
// Returns empty string if the result is invalid.
func slugifyOPML(s string) string {
	slug := strings.ToLower(s)
	slug = nonAlphanumRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if !slugRe.MatchString(slug) {
		return ""
	}
	return slug
}

// ImportOPML creates feeds (and optionally channels) from an uploaded OPML file.
// Existing feeds matched by URL are not overwritten — OPML carries no config.
func (h *SettingsHandler) ImportOPML(c *echo.Context) error {
	ctx := c.Request().Context()

	file, err := c.FormFile("opml_file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "OPML file is required")
	}
	if file.Size > maxImportSize {
		return echo.NewHTTPError(http.StatusBadRequest, "OPML file too large (max 5 MB)")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not open uploaded file")
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, maxImportSize))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not read uploaded file")
	}

	var doc opmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		slog.Warn("opml import: xml unmarshal failed", "err", err)
		return echo.NewHTTPError(http.StatusBadRequest, "invalid OPML format")
	}

	parsed := collectOPMLFeeds(doc.Body.Outlines, "")

	// Load existing feeds for upsert matching.
	existingFeeds, err := h.feeds.List(ctx)
	if err != nil {
		slog.Error("opml import: list feeds", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load existing feeds")
	}
	feedByURL := make(map[string]*feed.Feed, len(existingFeeds))
	for _, f := range existingFeeds {
		feedByURL[f.URL] = f
	}

	// importedFeedID maps URL → feed ID for channel membership below.
	importedFeedID := make(map[string]string, len(parsed))

	for _, pf := range parsed {
		if pf.url == "" {
			continue
		}
		if err := validateFeedURL(pf.url); err != nil {
			slog.Warn("opml import: skipping feed with invalid URL", "url", pf.url)
			continue
		}

		if existing, ok := feedByURL[pf.url]; ok {
			// Feed already exists — do not overwrite its settings.
			importedFeedID[pf.url] = existing.ID
			continue
		}

		// Create new feed with sensible defaults.
		created, err := h.feeds.Create(ctx, &feed.Feed{
			Name:                pf.name,
			URL:                 pf.url,
			Enabled:             true,
			PollIntervalSeconds: 3600,
			ScrapeFullContent:   false,
			ScrapeMethod:        feed.ScrapeMethodReadability,
			ScrapeSelectorType:  feed.SelectorTypeCSS,
			ImageMode:           feed.ImageModeRandom,
		})
		if err != nil {
			slog.Error("opml import: create feed", "url", pf.url, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create feed: "+pf.url)
		}
		importedFeedID[pf.url] = created.ID
		h.poller.StartFeed(created)
		h.poller.ForceRefresh(created.ID)
	}

	// Group parsed feeds by folder name and create/upsert channels.
	type folderFeeds struct {
		urls []string
	}
	folders := make(map[string]*folderFeeds)
	var folderOrder []string
	for _, pf := range parsed {
		if pf.folderName == "" {
			continue
		}
		if _, ok := importedFeedID[pf.url]; !ok {
			continue // feed was skipped (invalid URL)
		}
		if _, exists := folders[pf.folderName]; !exists {
			folders[pf.folderName] = &folderFeeds{}
			folderOrder = append(folderOrder, pf.folderName)
		}
		folders[pf.folderName].urls = append(folders[pf.folderName].urls, pf.url)
	}

	if len(folderOrder) > 0 {
		existingChannels, err := h.channels.List(ctx)
		if err != nil {
			slog.Error("opml import: list channels", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load existing channels")
		}
		channelBySlug := make(map[string]*channel.Channel, len(existingChannels))
		for _, ch := range existingChannels {
			channelBySlug[ch.Slug] = ch
		}

		for _, folderName := range folderOrder {
			slug := slugifyOPML(folderName)
			if slug == "" {
				slog.Warn("opml import: skipping folder with un-slugifiable name", "name", folderName)
				continue
			}

			var channelID string
			if existing, ok := channelBySlug[slug]; ok {
				channelID = existing.ID
			} else {
				created, err := h.channels.Create(ctx, &channel.Channel{
					Name: folderName,
					Slug: slug,
				})
				if err != nil {
					slog.Error("opml import: create channel", "slug", slug, "err", err)
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to create channel: "+slug)
				}
				channelID = created.ID
			}

			// Add feeds to channel (AddFeed is idempotent via ON CONFLICT DO NOTHING).
			for _, feedURL := range folders[folderName].urls {
				fid, ok := importedFeedID[feedURL]
				if !ok {
					continue
				}
				if err := h.channels.AddFeed(ctx, channelID, fid); err != nil {
					slog.Error("opml import: add feed to channel", "channel_id", channelID, "feed_id", fid, "err", err)
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to add feed to channel")
				}
			}
		}
	}

	slog.Info("opml import completed", "feeds_parsed", len(parsed), "folders", len(folderOrder))
	return redirect(c, "/settings")
}
