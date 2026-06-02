package handler

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/laserfeed/laserfeed/internal/atom"
	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

// atomCacheTTL bounds how stale a generated feed may be. RSS readers poll
// infrequently and feeds only change on poll cycles, so a short TTL collapses
// bursts of reader requests (and search-engine/aggregator crawls) into a single
// DB read + XML marshal without noticeably delaying new articles.
const atomCacheTTL = 60 * time.Second

type atomCacheEntry struct {
	body    []byte
	expires time.Time
}

// atomCache is a small in-memory TTL cache of rendered Atom output, keyed by
// channel slug (the all-feed uses a reserved key). It exists because a popular
// channel URL is hit repeatedly by every subscribed reader, and regenerating
// identical XML from the database on each request is wasteful.
type atomCache struct {
	mu      sync.RWMutex
	entries map[string]atomCacheEntry
}

func newAtomCache() *atomCache {
	return &atomCache{entries: make(map[string]atomCacheEntry)}
}

func (c *atomCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.body, true
}

func (c *atomCache) set(key string, body []byte) {
	c.mu.Lock()
	c.entries[key] = atomCacheEntry{body: body, expires: time.Now().Add(atomCacheTTL)}
	c.mu.Unlock()
}

const allFeedCacheKey = "\x00all"

type FeedOutHandler struct {
	channels   channel.Repository
	articles   article.Repository
	feeds      feed.Repository
	appBaseURL string
	cache      *atomCache
}

func NewFeedOutHandler(channels channel.Repository, articles article.Repository, feeds feed.Repository, appBaseURL string) *FeedOutHandler {
	return &FeedOutHandler{
		channels:   channels,
		articles:   articles,
		feeds:      feeds,
		appBaseURL: appBaseURL,
		cache:      newAtomCache(),
	}
}

func (h *FeedOutHandler) ChannelFeed(c *echo.Context) error {
	ctx := c.Request().Context()
	slug := c.Param("slug")

	if cached, ok := h.cache.get(slug); ok {
		return c.Blob(http.StatusOK, "application/atom+xml; charset=utf-8", cached)
	}

	ch, err := h.channels.GetBySlug(ctx, slug)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}

	channelFeeds, err := h.channels.ListFeeds(ctx, ch.ID)
	if err != nil {
		slog.Error("list channel feeds for output", "channel_id", ch.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
	}

	feedsByID := make(map[string]*feed.Feed, len(channelFeeds))
	feedIDs := make([]string, len(channelFeeds))
	for i, f := range channelFeeds {
		feedIDs[i] = f.ID
		feedsByID[f.ID] = f
	}

	arts, err := h.articles.ListByFeedIDs(ctx, feedIDs, 100, 0)
	if err != nil {
		slog.Error("list articles for channel feed", "channel_id", ch.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load articles")
	}

	atomBytes, err := atom.GenerateAtom(ch, arts, feedsByID, h.appBaseURL)
	if err != nil {
		slog.Error("generate atom feed", "channel_id", ch.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate feed")
	}

	h.cache.set(slug, atomBytes)
	return c.Blob(http.StatusOK, "application/atom+xml; charset=utf-8", atomBytes)
}

func (h *FeedOutHandler) AllFeed(c *echo.Context) error {
	ctx := c.Request().Context()

	if cached, ok := h.cache.get(allFeedCacheKey); ok {
		return c.Blob(http.StatusOK, "application/atom+xml; charset=utf-8", cached)
	}

	arts, err := h.articles.ListRecent(ctx, 100, 0)
	if err != nil {
		slog.Error("list recent articles for all feed", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load articles")
	}

	// Load only the feeds referenced by these articles, not the entire pool.
	feedIDs := make([]string, 0, len(arts))
	seen := make(map[string]struct{}, len(arts))
	for _, a := range arts {
		if _, ok := seen[a.FeedID]; ok {
			continue
		}
		seen[a.FeedID] = struct{}{}
		feedIDs = append(feedIDs, a.FeedID)
	}
	refFeeds, err := h.feeds.ListByIDs(ctx, feedIDs)
	if err != nil {
		slog.Error("list feeds for all-feed atom", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feeds")
	}
	feedsByID := make(map[string]*feed.Feed, len(refFeeds))
	for _, f := range refFeeds {
		feedsByID[f.ID] = f
	}

	allCh := &channel.Channel{
		Name: "LaserFeed — All",
		Slug: "all",
	}

	atomBytes, err := atom.GenerateAtom(allCh, arts, feedsByID, h.appBaseURL)
	if err != nil {
		slog.Error("generate all-feeds atom", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate feed")
	}

	h.cache.set(allFeedCacheKey, atomBytes)
	return c.Blob(http.StatusOK, "application/atom+xml; charset=utf-8", atomBytes)
}
