package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
	"github.com/laserfeed/laserfeed/internal/poller"
	"github.com/laserfeed/laserfeed/web/templates/pages"
)

type FeedHandler struct {
	feeds    feed.Repository
	articles article.Repository
	rules    filterrule.Repository
	poller   *poller.Manager
}

func NewFeedHandler(feeds feed.Repository, articles article.Repository, rules filterrule.Repository, pm *poller.Manager) *FeedHandler {
	return &FeedHandler{feeds: feeds, articles: articles, rules: rules, poller: pm}
}

func (h *FeedHandler) List(c *echo.Context) error {
	feeds, err := h.feeds.List(c.Request().Context())
	if err != nil {
		slog.Error("list feeds", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feeds")
	}
	return pages.FeedList(csrfToken(c), feeds).Render(c.Request().Context(), c.Response())
}

func (h *FeedHandler) Create(c *echo.Context) error {
	ctx := c.Request().Context()

	feedURL := strings.TrimSpace(c.FormValue("url"))
	if err := validateFeedURL(feedURL); err != nil {
		return err
	}

	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if len(name) > 255 {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be 255 characters or fewer")
	}

	pollInterval, _ := strconv.Atoi(c.FormValue("poll_interval_seconds"))
	if pollInterval < 60 {
		pollInterval = 3600
	}

	imageMode := feed.NormalizeImageMode(c.FormValue("image_mode"))

	f := &feed.Feed{
		Name:                name,
		URL:                 feedURL,
		Enabled:             true,
		PollIntervalSeconds: pollInterval,
		ScrapeMethod:        feed.ScrapeMethodReadability,
		ScrapeSelectorType:  feed.SelectorTypeCSS,
		ImageMode:           imageMode,
	}

	created, err := h.feeds.Create(ctx, f)
	if err != nil {
		slog.Error("create feed", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create feed")
	}

	h.poller.StartFeed(created)
	h.poller.ForceRefresh(created.ID)

	return redirect(c, "/feeds")
}

func (h *FeedHandler) Edit(c *echo.Context) error {
	ctx := c.Request().Context()
	f, err := h.feeds.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}

	var stats *article.ScrapeStats
	if f.ScrapeFullContent {
		stats, err = h.articles.GetScrapeStats(ctx, f.ID)
		if err != nil {
			slog.Error("get scrape stats", "feed_id", f.ID, "err", err)
		}
	}

	return pages.FeedEdit(csrfToken(c), f, stats).Render(ctx, c.Response())
}

func (h *FeedHandler) Update(c *echo.Context) error {
	ctx := c.Request().Context()
	f, err := h.feeds.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}

	feedURL := strings.TrimSpace(c.FormValue("url"))
	if err := validateFeedURL(feedURL); err != nil {
		return err
	}

	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if len(name) > 255 {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be 255 characters or fewer")
	}

	pollInterval, _ := strconv.Atoi(c.FormValue("poll_interval_seconds"))
	if pollInterval < 60 {
		pollInterval = 3600
	}

	scrapeMaxAge, _ := strconv.Atoi(c.FormValue("scrape_max_age_days"))
	if scrapeMaxAge < 0 {
		scrapeMaxAge = 0
	}

	retentionMaxItems, _ := strconv.Atoi(c.FormValue("retention_max_items"))
	if retentionMaxItems < 0 {
		retentionMaxItems = 0
	}
	retentionMaxHours, _ := strconv.Atoi(c.FormValue("retention_max_hours"))
	if retentionMaxHours < 0 {
		retentionMaxHours = 0
	}

	f.Name = name
	f.URL = feedURL
	f.Enabled = c.FormValue("enabled") == "true"
	f.PollIntervalSeconds = pollInterval
	f.ScrapeFullContent = c.FormValue("scrape_full_content") == "true"
	f.ScrapeMaxAgeDays = scrapeMaxAge
	f.RetentionMaxItems = retentionMaxItems
	f.RetentionMaxHours = retentionMaxHours
	f.ImageMode = feed.NormalizeImageMode(c.FormValue("image_mode"))
	f.ScrapeMethod = feed.NormalizeScrapeMethod(c.FormValue("scrape_method"))
	f.ScrapeSelectorType = feed.NormalizeSelectorType(c.FormValue("scrape_selector_type"))

	var perr error
	if f.UserAgent, perr = optionalStr(c, "user_agent", 500); perr != nil {
		return perr
	}
	if f.ScrapeSelector, perr = optionalStr(c, "scrape_selector", 1000); perr != nil {
		return perr
	}
	if f.ScrapeCookies, perr = optionalStr(c, "scrape_cookies", 8192); perr != nil {
		return perr
	}
	if f.ScrapeStripSelectors, perr = optionalStr(c, "scrape_strip_selectors", 4096); perr != nil {
		return perr
	}
	if f.ScrapePageStripSelectors, perr = optionalStr(c, "scrape_page_strip_selectors", 4096); perr != nil {
		return perr
	}
	if ph := strings.TrimSpace(c.FormValue("placeholder_image_url")); ph != "" {
		if err := validateFeedURL(ph); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid placeholder image URL")
		}
		f.PlaceholderImageURL = &ph
	} else {
		f.PlaceholderImageURL = nil
	}

	updated, err := h.feeds.Update(ctx, f)
	if err != nil {
		slog.Error("update feed", "feed_id", f.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update feed")
	}

	if updated.Enabled {
		h.poller.StartFeed(updated)
	} else {
		h.poller.StopFeed(updated.ID)
	}

	return redirect(c, "/feeds/"+f.ID+"/edit")
}

func (h *FeedHandler) Delete(c *echo.Context) error {
	id := c.Param("id")
	h.poller.StopFeed(id)
	if err := h.feeds.Delete(c.Request().Context(), id); err != nil {
		slog.Error("delete feed", "feed_id", id, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete feed")
	}
	return redirect(c, "/feeds")
}

func (h *FeedHandler) Refresh(c *echo.Context) error {
	h.poller.ForceRefresh(c.Param("id"))
	return redirect(c, "/feeds")
}

func (h *FeedHandler) RefreshAll(c *echo.Context) error {
	h.poller.RefreshAll()
	return redirect(c, "/feeds")
}

func (h *FeedHandler) Preview(c *echo.Context) error {
	ctx := c.Request().Context()
	f, err := h.feeds.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}

	mode := c.QueryParam("mode") // "raw" or "" (filtered)
	showRaw := mode == "raw"

	arts, err := h.articles.ListByFeedID(ctx, f.ID, showRaw, 100, 0)
	if err != nil {
		slog.Error("preview feed articles", "feed_id", f.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load articles")
	}

	filterRules, err := h.rules.ListByFeedID(ctx, f.ID)
	if err != nil {
		slog.Error("preview feed rules", "feed_id", f.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load filter rules")
	}

	return pages.FeedPreview(csrfToken(c), f, arts, filterRules, showRaw).Render(ctx, c.Response())
}

// Scrape triggers a background re-scrape of all articles for this feed.
func (h *FeedHandler) Scrape(c *echo.Context) error {
	id := c.Param("id")
	f, err := h.feeds.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}
	if !f.ScrapeFullContent {
		return echo.NewHTTPError(http.StatusBadRequest, "scraping is not enabled for this feed")
	}
	h.poller.ReScrapeArticles(id)
	return redirect(c, "/feeds/"+id+"/edit")
}

// PurgeScrape clears all scraped content for this feed.
func (h *FeedHandler) PurgeScrape(c *echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if err := h.articles.PurgeScrapeContent(ctx, id); err != nil {
		slog.Error("purge scrape content", "feed_id", id, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to purge content")
	}
	return redirect(c, "/feeds/"+id+"/edit")
}

// optionalStr reads a form field as a trimmed *string: empty (or whitespace-only)
// becomes nil; non-empty is bounds-checked and returned as a heap pointer.
func optionalStr(c *echo.Context, field string, maxLen int) (*string, error) {
	v := strings.TrimSpace(c.FormValue(field))
	if v == "" {
		return nil, nil
	}
	if len(v) > maxLen {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("%s must be %d characters or fewer", field, maxLen))
	}
	return &v, nil
}

func validateFeedURL(rawURL string) error {
	if rawURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "URL is required")
	}
	if len(rawURL) > 2048 {
		return echo.NewHTTPError(http.StatusBadRequest, "URL must be 2048 characters or fewer")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return echo.NewHTTPError(http.StatusBadRequest, "URL must start with http:// or https://")
	}
	if u.Host == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "URL must include a host")
	}
	if u.User != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "URL must not contain credentials")
	}
	return nil
}
