package poller

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
	"github.com/laserfeed/laserfeed/internal/domain/settings"
	"github.com/laserfeed/laserfeed/internal/filter"
	"github.com/laserfeed/laserfeed/internal/scraper"
	"github.com/mmcdole/gofeed"
)

// itemGUID returns the stable identifier for a feed item, falling back to the
// link when the feed provides no GUID.
func itemGUID(item *gofeed.Item) string {
	if item.GUID != "" {
		return item.GUID
	}
	return item.Link
}

// rulesMatchContent reports whether any rule matches on the content field.
func rulesMatchContent(rules []*filterrule.FilterRule) bool {
	for _, r := range rules {
		if r.MatchField == filterrule.MatchFieldContent {
			return true
		}
	}
	return false
}

type Stores struct {
	Feeds         feed.Repository
	Articles      article.Repository
	FilterRules   filterrule.Repository
	Settings      settings.Repository
	AppBaseURL    string // used to build absolute URLs for built-in placeholder images
	JSRenderWSURL string // optional CDP endpoint for JS rendering (JS_RENDER_WS_URL)
}

// builtinSVGs is the ordered list of built-in placeholder image filenames.
var builtinSVGs = []string{
	"laserfeed-placeholder.svg",
	"laserfeed-placeholder-2.svg",
	"laserfeed-placeholder-3.svg",
	"laserfeed-placeholder-4.svg",
	"laserfeed-placeholder-5.svg",
	"laserfeed-placeholder-6.svg",
	"laserfeed-placeholder-7.svg",
	"laserfeed-placeholder-8.svg",
	"laserfeed-placeholder-9.svg",
}

// resolveBuiltinFile returns the SVG filename to use for a given article.
// When file is "__rotate__" the choice is deterministic per article GUID so
// the same article always gets the same image, but different articles are
// spread across the available built-in images.
func resolveBuiltinFile(file, guid string) string {
	if file != "__rotate__" {
		return file
	}
	h := fnv.New32a()
	// hash.Hash.Write never returns an error, so the result is ignored intentionally.
	_, _ = h.Write([]byte(guid))
	return builtinSVGs[h.Sum32()%uint32(len(builtinSVGs))]
}

// maxConcurrentScrapes bounds how many article pages a single feed poll will
// fetch in parallel, so a feed with many articles ingests quickly without
// opening an unbounded number of outbound connections.
const maxConcurrentScrapes = 4

// pollTimeout is a generous outer backstop on a single poll cycle so a wedged
// DB operation or stuck connection cannot pin a feed goroutine indefinitely.
// The feed fetch (30s) and per-article scrape (15s) keep their own tighter limits.
const pollTimeout = 5 * time.Minute

// redactURLUserinfo strips any user:password@ component from a URL before it is
// logged. Feed URLs are validated to reject credentials on input, but this is
// defense-in-depth so a credential can never leak into the logs.
func redactURLUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// resolveScrapeParams merges per-feed overrides on top of global defaults into
// the options passed to the scraper.
func resolveScrapeParams(f *feed.Feed, globalUA string) scraper.ScrapeOptions {
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
	var stripSelectors []string
	if f.ScrapeStripSelectors != nil {
		for _, line := range strings.Split(*f.ScrapeStripSelectors, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				stripSelectors = append(stripSelectors, trimmed)
			}
		}
	}
	var pageStripSelectors []string
	if f.ScrapePageStripSelectors != nil {
		for _, line := range strings.Split(*f.ScrapePageStripSelectors, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				pageStripSelectors = append(pageStripSelectors, trimmed)
			}
		}
	}
	return scraper.ScrapeOptions{
		UserAgent:          ua,
		Method:             string(f.ScrapeMethod),
		Selector:           sel,
		SelectorType:       string(f.ScrapeSelectorType),
		Cookies:            ck,
		StripSelectors:     stripSelectors,
		PageStripSelectors: pageStripSelectors,
		RenderJS:           f.ScrapeRenderJS,
	}
}

func pollOnce(ctx context.Context, feedID string, stores Stores, sc *scraper.Scraper) {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

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
	// Per-feed placeholder URL takes priority; fall back to the global setting.
	placeholderURL := globalSettings.PlaceholderImageURL
	if f.PlaceholderImageURL != nil {
		placeholderURL = *f.PlaceholderImageURL
	}
	// For "builtin" mode the final URL is resolved per-article inside the loop
	// (the __rotate__ option picks a different SVG per article based on GUID).
	// Capture the builtin filename here so the loop can use it.
	builtinFile := ""
	if imageMode == "builtin" {
		builtinFile = globalSettings.BuiltinPlaceholder
		if builtinFile == "" {
			builtinFile = "laserfeed-placeholder.svg"
		}
	}

	now := time.Now()
	result, err := sc.FetchFeed(ctx, f.URL, sp.UserAgent, f.PollETag, f.PollLastModified)
	if err != nil {
		errStr := err.Error()
		if statusErr := stores.Feeds.UpdatePollStatus(ctx, feedID, now, &errStr); statusErr != nil {
			slog.Warn("poller: persist fetch error status", "feed_id", feedID, "err", statusErr)
		}
		slog.Error("poller: fetch feed", "feed_id", feedID, "url", redactURLUserinfo(f.URL), "err", err)
		return
	}

	if err := stores.Feeds.UpdatePollStatus(ctx, feedID, now, nil); err != nil {
		slog.Warn("poller: persist ok status", "feed_id", feedID, "err", err)
	}

	// Conditional GET: nothing changed since last poll — skip re-ingesting items,
	// but still run retention so time-based cleanup keeps happening.
	if result.NotModified {
		slog.Debug("poller: feed not modified", "feed_id", feedID)
		runRetention(ctx, f, stores, globalSettings)
		return
	}

	// Store fresh validators for the next conditional request.
	if err := stores.Feeds.UpdatePollValidators(ctx, feedID, result.ETag, result.LastModified); err != nil {
		slog.Warn("poller: persist validators", "feed_id", feedID, "err", err)
	}
	parsedFeed := result.Feed

	rules, err := stores.FilterRules.ListByFeedID(ctx, feedID)
	if err != nil {
		slog.Error("poller: list rules", "feed_id", feedID, "err", err)
		rules = nil
	}

	// Skip items that retention already deleted; without this, an item deleted
	// by count/age retention but still listed in the source feed would be
	// re-ingested (and re-scraped) every poll, then deleted again at the end of
	// the cycle. Failure to load tombstones fails open (items are re-ingested).
	tombstoned, err := stores.Articles.GetTombstonedGUIDs(ctx, feedID)
	if err != nil {
		slog.Warn("poller: get tombstoned guids", "feed_id", feedID, "err", err)
		tombstoned = map[string]bool{}
	}
	items := parsedFeed.Items
	if len(tombstoned) > 0 {
		items = make([]*gofeed.Item, 0, len(parsedFeed.Items))
		for _, item := range parsedFeed.Items {
			if !tombstoned[itemGUID(item)] {
				items = append(items, item)
			}
		}
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

	// When content-matching rules exist, filter evaluation for already-scraped
	// articles must see the stored full content — the feed item only carries an
	// excerpt, so is_filtered_out would otherwise flip depending on whether the
	// article was scraped this cycle or a previous one.
	storedContents := map[string]string{}
	if f.ScrapeFullContent && rulesMatchContent(rules) {
		if contents, err := stores.Articles.GetScrapedContents(ctx, feedID); err == nil {
			storedContents = contents
		} else {
			slog.Warn("poller: get scraped contents", "feed_id", feedID, "err", err)
		}
	}

	// Phase 1: decide which items need a network scrape this cycle.
	type itemWork struct {
		guid        string
		needsScrape bool
		scraped     string
		scrapeErr   error
	}
	works := make([]itemWork, len(items))
	for i, item := range items {
		guid := itemGUID(item)
		works[i] = itemWork{
			guid:        guid,
			needsScrape: f.ScrapeFullContent && item.Link != "" && !scrapedGUIDs[guid],
		}
	}

	// Phase 2: scrape the pages that need it in parallel, bounded by a
	// semaphore. Each goroutine writes only to its own works[i], so no locking
	// is needed. A single slow page no longer stalls the rest of the cycle.
	if f.ScrapeFullContent {
		sem := make(chan struct{}, maxConcurrentScrapes)
		var wg sync.WaitGroup
		for i := range works {
			if !works[i].needsScrape {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(w *itemWork, link string) {
				defer wg.Done()
				defer func() { <-sem }()
				w.scraped, w.scrapeErr = sc.ScrapeContent(ctx, link, sp)
			}(&works[i], items[i].Link)
		}
		wg.Wait()
	}

	// Phase 3: build and upsert each article from its (possibly scraped) content.
	for i, item := range items {
		w := &works[i]
		guid := w.guid

		var content string
		var scrapeStatus article.ScrapeStatus
		var scrapeError string

		switch {
		case w.needsScrape && w.scrapeErr != nil:
			slog.Warn("poller: scrape content", "url", item.Link, "err", w.scrapeErr)
			scrapeStatus = article.ScrapeStatusFailed
			scrapeError = w.scrapeErr.Error()
			content = scraper.SanitizeHTML(item.Content)
		case w.needsScrape && strings.TrimSpace(w.scraped) == "":
			if sp.Method == "selector" {
				scrapeError = fmt.Sprintf("selector %q matched no content", sp.Selector)
			} else {
				scrapeError = "readability could not extract content from the page"
			}
			slog.Warn("poller: scrape empty", "url", item.Link, "reason", scrapeError)
			scrapeStatus = article.ScrapeStatusFailed
			content = scraper.SanitizeHTML(item.Content)
		case w.needsScrape:
			scrapeStatus = article.ScrapeStatusSuccess
			content = w.scraped
		case scrapedGUIDs[guid]:
			// Already scraped — upsert preserves the stored content. Evaluate
			// filters against that stored content when it was loaded (i.e. when
			// content rules exist), not the feed excerpt.
			scrapeStatus = article.ScrapeStatusSuccess
			if stored, ok := storedContents[guid]; ok {
				content = stored
			} else {
				content = scraper.SanitizeHTML(item.Content)
			}
		default:
			scrapeStatus = article.ScrapeStatusNone
			content = scraper.SanitizeHTML(item.Content)
		}

		description := scraper.SanitizeHTML(item.Description)

		// Resolve the effective imageMode and placeholderURL for this article.
		// "builtin" is expanded here because __rotate__ needs the per-article GUID.
		effectiveMode := imageMode
		effectivePlaceholder := placeholderURL
		if builtinFile != "" {
			effectiveMode = "placeholder"
			joined, joinErr := url.JoinPath(stores.AppBaseURL, "/static/images/", resolveBuiltinFile(builtinFile, guid))
			if joinErr == nil {
				effectivePlaceholder = joined
			} else {
				slog.Warn("poller: build builtin placeholder url", "err", joinErr)
				effectivePlaceholder = stores.AppBaseURL + "/static/images/" + resolveBuiltinFile(builtinFile, guid)
			}
		}

		thumbnail := scraper.ExtractThumbnail(item, description, content, effectiveMode, effectivePlaceholder, guid)

		publishedAt := now
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

	runRetention(ctx, f, stores, globalSettings)

	slog.Info("poller: done", "feed_id", feedID, "items", len(items),
		"tombstoned_skipped", len(parsedFeed.Items)-len(items))
}

// runRetention applies a feed's content-purge and article-retention policies.
// Safe to call on every poll, including 304 (not-modified) cycles.
func runRetention(ctx context.Context, f *feed.Feed, stores Stores, gs *settings.Settings) {
	if f.ScrapeFullContent && f.ScrapeMaxAgeDays > 0 {
		if err := stores.Articles.PurgeOldScrapeContent(ctx, f.ID, f.ScrapeMaxAgeDays); err != nil {
			slog.Warn("poller: purge old scrape content", "feed_id", f.ID, "err", err)
		}
	}

	// Item-count retention: per-feed value overrides global when set.
	maxItems := gs.MaxArticlesPerFeed
	if f.RetentionMaxItems > 0 {
		maxItems = f.RetentionMaxItems
	}
	if maxItems > 0 {
		if err := stores.Articles.DeleteOldest(ctx, f.ID, maxItems); err != nil {
			slog.Warn("poller: delete oldest", "feed_id", f.ID, "err", err)
		}
	}
	// Time-based retention: per-feed only.
	if f.RetentionMaxHours > 0 {
		if err := stores.Articles.DeleteOlderThan(ctx, f.ID, f.RetentionMaxHours); err != nil {
			slog.Warn("poller: delete older than", "feed_id", f.ID, "hours", f.RetentionMaxHours, "err", err)
		}
	}
}
