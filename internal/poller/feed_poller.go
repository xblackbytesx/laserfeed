package poller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// scrapeParams holds the resolved scraping configuration for a single feed,
// merging per-feed overrides on top of global defaults.
type scrapeParams struct {
	userAgent    string
	selector     string
	selectorType string
	cookies      string
}

func resolveScrapeParams(f *feed.Feed, globalUA string) scrapeParams {
	ua := globalUA
	if f.UserAgent != nil && *f.UserAgent != "" {
		ua = *f.UserAgent
	}
	sel := ""
	if f.ScrapeSelector != nil {
		sel = *f.ScrapeSelector
	}
	ck := ""
	if f.ScrapeCookies != nil {
		ck = *f.ScrapeCookies
	}
	return scrapeParams{
		userAgent:    ua,
		selector:     sel,
		selectorType: string(f.ScrapeSelectorType),
		cookies:      ck,
	}
}

func pollOnce(ctx context.Context, feedID string, stores Stores, sc *scraper.Scraper) {
	f, err := stores.Feeds.GetByID(ctx, feedID)
	if err != nil {
		slog.Error("poller: get feed", "feed_id", feedID, "err", err)
		return
	}

	globalSettings, err := stores.Settings.Get(ctx)
	if err != nil {
		slog.Error("poller: get settings", "err", err)
		return
	}

	sp := resolveScrapeParams(f, globalSettings.UserAgent)

	imageMode := string(globalSettings.ImageMode)
	if string(f.ImageMode) != "" {
		imageMode = string(f.ImageMode)
	}
	placeholderURL := ""
	if f.PlaceholderImageURL != nil {
		placeholderURL = *f.PlaceholderImageURL
	}

	parsedFeed, err := sc.FetchFeed(ctx, f.URL, sp.userAgent)
	now := time.Now()
	if err != nil {
		errStr := err.Error()
		_ = stores.Feeds.UpdatePollStatus(ctx, feedID, now, &errStr)
		slog.Error("poller: fetch feed", "feed_id", feedID, "url", f.URL, "err", err)
		return
	}

	_ = stores.Feeds.UpdatePollStatus(ctx, feedID, now, nil)

	rules, err := stores.FilterRules.ListByFeedID(ctx, feedID)
	if err != nil {
		slog.Error("poller: list rules", "feed_id", feedID, "err", err)
		rules = nil
	}

	// Pre-fetch GUIDs that already have successfully scraped content so we skip
	// redundant HTTP requests for them on subsequent polls.
	scrapedGUIDs := map[string]bool{}
	if f.ScrapeFullContent {
		if guids, err := stores.Articles.GetScrapedGUIDs(ctx, feedID); err == nil {
			scrapedGUIDs = guids
		} else {
			slog.Warn("poller: get scraped guids", "feed_id", feedID, "err", err)
		}
	}

	for _, item := range parsedFeed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		var content string
		var scrapeStatus article.ScrapeStatus
		var scrapeError string

		if f.ScrapeFullContent && item.Link != "" && !scrapedGUIDs[guid] {
			scraped, err := sc.ScrapeContent(ctx, item.Link, sp.userAgent, sp.selector, sp.selectorType, sp.cookies)
			switch {
			case err != nil:
				slog.Warn("poller: scrape content", "url", item.Link, "err", err)
				scrapeStatus = article.ScrapeStatusFailed
				scrapeError = err.Error()
				content = scraper.SanitizeHTML(item.Content)
			case strings.TrimSpace(scraped) == "":
				if sp.selector != "" {
					scrapeError = fmt.Sprintf("selector %q matched no content", sp.selector)
				} else {
					scrapeError = "no content could be extracted from the page"
				}
				slog.Warn("poller: scrape empty", "url", item.Link, "reason", scrapeError)
				scrapeStatus = article.ScrapeStatusFailed
				content = scraper.SanitizeHTML(item.Content)
			default:
				scrapeStatus = article.ScrapeStatusSuccess
				content = scraped
			}
		} else if scrapedGUIDs[guid] {
			// Already successfully scraped — the upsert CASE WHEN will preserve
			// the stored content; just carry the RSS content as a placeholder.
			scrapeStatus = article.ScrapeStatusSuccess
			content = scraper.SanitizeHTML(item.Content)
		} else {
			scrapeStatus = article.ScrapeStatusNone
			content = scraper.SanitizeHTML(item.Content)
		}

		description := scraper.SanitizeHTML(item.Description)
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
			FeedID:       feedID,
			GUID:         guid,
			Title:        item.Title,
			URL:          item.Link,
			Author:       author,
			Description:  description,
			Content:      content,
			ThumbnailURL: thumbnail,
			PublishedAt:  publishedAt,
			FetchedAt:    now,
			ScrapeStatus: scrapeStatus,
			ScrapeError:  scrapeError,
		}
		a.IsFilteredOut = filter.Apply(a, rules)

		if err := stores.Articles.Upsert(ctx, a); err != nil {
			slog.Error("poller: upsert article", "guid", guid, "err", err)
		}
	}

	// Enforce per-feed scrape content retention
	if f.ScrapeFullContent && f.ScrapeMaxAgeDays > 0 {
		if err := stores.Articles.PurgeOldScrapeContent(ctx, feedID, f.ScrapeMaxAgeDays); err != nil {
			slog.Warn("poller: purge old scrape content", "feed_id", feedID, "err", err)
		}
	}

	// Trim old articles
	if globalSettings.MaxArticlesPerFeed > 0 {
		if err := stores.Articles.DeleteOldest(ctx, feedID, globalSettings.MaxArticlesPerFeed); err != nil {
			slog.Warn("poller: delete oldest", "feed_id", feedID, "err", err)
		}
	}

	slog.Info("poller: done", "feed_id", feedID, "items", len(parsedFeed.Items))
}
