package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
)

type backupDoc struct {
	Version    int             `json:"version"`
	ExportedAt string          `json:"exported_at"`
	Feeds      []backupFeed    `json:"feeds"`
	Channels   []backupChannel `json:"channels"`
}

type backupFeed struct {
	Name                string       `json:"name"`
	URL                 string       `json:"url"`
	Enabled             bool         `json:"enabled"`
	PollIntervalSeconds int          `json:"poll_interval_seconds"`
	UserAgent           *string      `json:"user_agent,omitempty"`
	ScrapeFullContent   bool         `json:"scrape_full_content"`
	ScrapeSelector      *string      `json:"scrape_selector,omitempty"`
	ScrapeSelectorType  string       `json:"scrape_selector_type"`
	ScrapeMaxAgeDays    int          `json:"scrape_max_age_days"`
	ScrapeCookies           *string      `json:"scrape_cookies,omitempty"`
	ScrapeStripSelectors    *string      `json:"scrape_strip_selectors,omitempty"`
	ImageMode               string       `json:"image_mode"`
	PlaceholderImageURL *string      `json:"placeholder_image_url,omitempty"`
	FilterRules         []backupRule `json:"filter_rules,omitempty"`
}

type backupRule struct {
	RuleType     string `json:"rule_type"`
	MatchField   string `json:"match_field"`
	MatchPattern string `json:"match_pattern"`
}

type backupChannel struct {
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Description string   `json:"description,omitempty"`
	FeedURLs    []string `json:"feed_urls"`
}

const maxImportSize = 5 << 20 // 5 MB

// Export streams the full configuration as a downloadable JSON file.
func (h *SettingsHandler) Export(c echo.Context) error {
	ctx := c.Request().Context()

	feeds, err := h.feeds.List(ctx)
	if err != nil {
		slog.Error("export: list feeds", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feeds")
	}

	channels, err := h.channels.List(ctx)
	if err != nil {
		slog.Error("export: list channels", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channels")
	}

	doc := backupDoc{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, f := range feeds {
		rules, err := h.filterRules.ListByFeedID(ctx, f.ID)
		if err != nil {
			slog.Error("export: list rules", "feed_id", f.ID, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load filter rules")
		}

		bf := backupFeed{
			Name:                 f.Name,
			URL:                  f.URL,
			Enabled:              f.Enabled,
			PollIntervalSeconds:  f.PollIntervalSeconds,
			UserAgent:            f.UserAgent,
			ScrapeFullContent:    f.ScrapeFullContent,
			ScrapeSelector:       f.ScrapeSelector,
			ScrapeSelectorType:   string(f.ScrapeSelectorType),
			ScrapeMaxAgeDays:     f.ScrapeMaxAgeDays,
			ScrapeCookies:        f.ScrapeCookies,
			ScrapeStripSelectors: f.ScrapeStripSelectors,
			ImageMode:            string(f.ImageMode),
			PlaceholderImageURL:  f.PlaceholderImageURL,
		}
		for _, r := range rules {
			bf.FilterRules = append(bf.FilterRules, backupRule{
				RuleType:     string(r.RuleType),
				MatchField:   string(r.MatchField),
				MatchPattern: r.MatchPattern,
			})
		}
		doc.Feeds = append(doc.Feeds, bf)
	}

	for _, ch := range channels {
		chFeeds, err := h.channels.ListFeeds(ctx, ch.ID)
		if err != nil {
			slog.Error("export: list channel feeds", "channel_id", ch.ID, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
		}

		bc := backupChannel{
			Name:        ch.Name,
			Slug:        ch.Slug,
			Description: ch.Description,
		}
		for _, f := range chFeeds {
			bc.FeedURLs = append(bc.FeedURLs, f.URL)
		}
		doc.Channels = append(doc.Channels, bc)
	}

	filename := fmt.Sprintf("laserfeed-backup-%s.json", time.Now().UTC().Format("2006-01-02"))
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+filename)
	c.Response().Header().Set("Content-Type", "application/json; charset=utf-8")

	enc := json.NewEncoder(c.Response().Writer)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		slog.Error("export: encode json", "err", err)
	}
	return nil
}

// Import upserts feeds (matched by URL) and channels (matched by slug) from a backup file.
func (h *SettingsHandler) Import(c echo.Context) error {
	ctx := c.Request().Context()

	file, err := c.FormFile("backup_file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "backup file is required")
	}
	if file.Size > maxImportSize {
		return echo.NewHTTPError(http.StatusBadRequest, "backup file too large (max 5 MB)")
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

	var doc backupDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	if doc.Version != 1 {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unsupported backup version %d", doc.Version))
	}

	// Build URL→feed and URL→id maps for upsert and channel membership.
	existingFeeds, err := h.feeds.List(ctx)
	if err != nil {
		slog.Error("import: list feeds", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load existing feeds")
	}
	feedByURL := make(map[string]*feed.Feed, len(existingFeeds))
	for _, f := range existingFeeds {
		feedByURL[f.URL] = f
	}

	importedFeedID := make(map[string]string)

	for _, bf := range doc.Feeds {
		if bf.URL == "" {
			continue
		}

		var feedID string

		if existing, ok := feedByURL[bf.URL]; ok {
			existing.Name = bf.Name
			existing.Enabled = bf.Enabled
			existing.PollIntervalSeconds = bf.PollIntervalSeconds
			existing.UserAgent = bf.UserAgent
			existing.ScrapeFullContent = bf.ScrapeFullContent
			existing.ScrapeSelector = bf.ScrapeSelector
			existing.ScrapeSelectorType = feed.SelectorType(bf.ScrapeSelectorType)
			existing.ScrapeMaxAgeDays = bf.ScrapeMaxAgeDays
			existing.ScrapeCookies = bf.ScrapeCookies
			existing.ScrapeStripSelectors = bf.ScrapeStripSelectors
			existing.ImageMode = feed.ImageMode(bf.ImageMode)
			existing.PlaceholderImageURL = bf.PlaceholderImageURL

			if _, err := h.feeds.Update(ctx, existing); err != nil {
				slog.Error("import: update feed", "url", bf.URL, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to update feed: "+bf.URL)
			}
			feedID = existing.ID
		} else {
			selectorType := feed.SelectorType(bf.ScrapeSelectorType)
			if selectorType == "" {
				selectorType = feed.SelectorTypeCSS
			}
			imageMode := feed.ImageMode(bf.ImageMode)
			if imageMode == "" {
				imageMode = feed.ImageModeExtract
			}

			created, err := h.feeds.Create(ctx, &feed.Feed{
				Name:                 bf.Name,
				URL:                  bf.URL,
				Enabled:              bf.Enabled,
				PollIntervalSeconds:  bf.PollIntervalSeconds,
				UserAgent:            bf.UserAgent,
				ScrapeFullContent:    bf.ScrapeFullContent,
				ScrapeSelector:       bf.ScrapeSelector,
				ScrapeSelectorType:   selectorType,
				ScrapeMaxAgeDays:     bf.ScrapeMaxAgeDays,
				ScrapeCookies:        bf.ScrapeCookies,
				ScrapeStripSelectors: bf.ScrapeStripSelectors,
				ImageMode:            imageMode,
				PlaceholderImageURL:  bf.PlaceholderImageURL,
			})
			if err != nil {
				slog.Error("import: create feed", "url", bf.URL, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to create feed: "+bf.URL)
			}
			feedID = created.ID
		}

		importedFeedID[bf.URL] = feedID

		if err := h.filterRules.DeleteAllByFeedID(ctx, feedID); err != nil {
			slog.Error("import: delete rules", "feed_id", feedID, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to replace filter rules")
		}
		for _, br := range bf.FilterRules {
			_, err := h.filterRules.Create(ctx, &filterrule.FilterRule{
				FeedID:       feedID,
				RuleType:     filterrule.RuleType(br.RuleType),
				MatchField:   filterrule.MatchField(br.MatchField),
				MatchPattern: br.MatchPattern,
			})
			if err != nil {
				slog.Error("import: create rule", "feed_id", feedID, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to create filter rule")
			}
		}
	}

	existingChannels, err := h.channels.List(ctx)
	if err != nil {
		slog.Error("import: list channels", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load existing channels")
	}
	channelBySlug := make(map[string]*channel.Channel, len(existingChannels))
	for _, ch := range existingChannels {
		channelBySlug[ch.Slug] = ch
	}

	for _, bc := range doc.Channels {
		if bc.Slug == "" {
			continue
		}

		var channelID string

		if existing, ok := channelBySlug[bc.Slug]; ok {
			existing.Name = bc.Name
			existing.Description = bc.Description
			if _, err := h.channels.Update(ctx, existing); err != nil {
				slog.Error("import: update channel", "slug", bc.Slug, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to update channel: "+bc.Slug)
			}
			channelID = existing.ID
		} else {
			created, err := h.channels.Create(ctx, &channel.Channel{
				Name:        bc.Name,
				Slug:        bc.Slug,
				Description: bc.Description,
			})
			if err != nil {
				slog.Error("import: create channel", "slug", bc.Slug, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to create channel: "+bc.Slug)
			}
			channelID = created.ID
		}

		currentFeeds, err := h.channels.ListFeeds(ctx, channelID)
		if err != nil {
			slog.Error("import: list channel feeds", "channel_id", channelID, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
		}
		for _, f := range currentFeeds {
			if err := h.channels.RemoveFeed(ctx, channelID, f.ID); err != nil {
				slog.Error("import: remove channel feed", "channel_id", channelID, "feed_id", f.ID, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to clear channel feeds")
			}
		}
		for _, feedURL := range bc.FeedURLs {
			fid, ok := importedFeedID[feedURL]
			if !ok {
				// Feed wasn't in the backup — check existing feeds.
				if f, ok := feedByURL[feedURL]; ok {
					fid = f.ID
				} else {
					slog.Warn("import: channel references unknown feed URL, skipping", "url", feedURL, "channel", bc.Slug)
					continue
				}
			}
			if err := h.channels.AddFeed(ctx, channelID, fid); err != nil {
				slog.Error("import: add feed to channel", "channel_id", channelID, "feed_id", fid, "err", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to add feed to channel")
			}
		}
	}

	slog.Info("import completed", "feeds", len(doc.Feeds), "channels", len(doc.Channels))
	return redirect(c, "/settings")
}
