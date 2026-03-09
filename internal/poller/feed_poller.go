package poller

import (
	"context"
	"log/slog"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
	"github.com/laserfeed/laserfeed/internal/domain/settings"
	"github.com/laserfeed/laserfeed/internal/filter"
	"github.com/laserfeed/laserfeed/internal/scraper"
)

type Stores struct {
	Feeds       feed.Repository
	Articles    article.Repository
	FilterRules filterrule.Repository
	Settings    settings.Repository
}

func pollOnce(ctx context.Context, feedID string, stores Stores, sc *scraper.Scraper) {
	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	f, err := stores.Feeds.GetByID(tctx, feedID)
	if err != nil {
		slog.Error("poller: get feed", "feed_id", feedID, "err", err)
		return
	}

	globalSettings, err := stores.Settings.Get(tctx)
	if err != nil {
		slog.Error("poller: get settings", "err", err)
		return
	}

	userAgent := globalSettings.UserAgent
	if f.UserAgent != nil && *f.UserAgent != "" {
		userAgent = *f.UserAgent
	}

	imageMode := string(globalSettings.ImageMode)
	if string(f.ImageMode) != "" {
		imageMode = string(f.ImageMode)
	}
	placeholderURL := ""
	if f.PlaceholderImageURL != nil {
		placeholderURL = *f.PlaceholderImageURL
	}

	parsedFeed, err := sc.FetchFeed(f.URL, userAgent)
	now := time.Now()
	if err != nil {
		errStr := err.Error()
		_ = stores.Feeds.UpdatePollStatus(tctx, feedID, now, &errStr)
		slog.Error("poller: fetch feed", "feed_id", feedID, "url", f.URL, "err", err)
		return
	}

	_ = stores.Feeds.UpdatePollStatus(tctx, feedID, now, nil)

	rules, err := stores.FilterRules.ListByFeedID(tctx, feedID)
	if err != nil {
		slog.Error("poller: list rules", "feed_id", feedID, "err", err)
		rules = nil
	}

	for _, item := range parsedFeed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		var content string
		if f.ScrapeFullContent && item.Link != "" {
			selector := ""
			selectorType := "css"
			if f.ScrapeSelector != nil {
				selector = *f.ScrapeSelector
			}
			if f.ScrapeSelectorType != "" {
				selectorType = string(f.ScrapeSelectorType)
			}
			scraped, err := sc.ScrapeContent(item.Link, userAgent, selector, selectorType)
			if err != nil {
				slog.Warn("poller: scrape content", "url", item.Link, "err", err)
				content = item.Content
			} else {
				content = scraped
			}
		} else {
			content = item.Content
		}

		description := item.Description

		thumbnail := scraper.ExtractThumbnail(item, description, content, imageMode, placeholderURL, guid)

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}

		a := &article.Article{
			FeedID:      feedID,
			GUID:        guid,
			Title:       item.Title,
			URL:         item.Link,
			Author:      author,
			Description: description,
			Content:     content,
			ThumbnailURL: thumbnail,
			PublishedAt: publishedAt,
			FetchedAt:   now,
		}
		a.IsFilteredOut = filter.Apply(a, rules)

		if err := stores.Articles.Upsert(tctx, a); err != nil {
			slog.Error("poller: upsert article", "guid", guid, "err", err)
		}
	}

	// Trim old articles
	if globalSettings.MaxArticlesPerFeed > 0 {
		if err := stores.Articles.DeleteOldest(tctx, feedID, globalSettings.MaxArticlesPerFeed); err != nil {
			slog.Warn("poller: delete oldest", "feed_id", feedID, "err", err)
		}
	}

	slog.Info("poller: done", "feed_id", feedID, "items", len(parsedFeed.Items))
}
