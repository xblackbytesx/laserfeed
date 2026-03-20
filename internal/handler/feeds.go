package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
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

func (h *FeedHandler) List(c echo.Context) error {
	feeds, err := h.feeds.List(c.Request().Context())
	if err != nil {
		slog.Error("list feeds", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feeds")
	}
	return pages.FeedList(csrfToken(c), feeds).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FeedHandler) Create(c echo.Context) error {
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

	imageMode := feed.ImageMode(c.FormValue("image_mode"))
	if imageMode == "" {
		imageMode = feed.ImageModeExtract
	}

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

func (h *FeedHandler) Edit(c echo.Context) error {
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

	return pages.FeedEdit(csrfToken(c), f, stats).Render(ctx, c.Response().Writer)
}

func (h *FeedHandler) Update(c echo.Context) error {
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

	f.Name = name
	f.URL = feedURL
	f.Enabled = c.FormValue("enabled") == "true"
	f.PollIntervalSeconds = pollInterval
	f.ScrapeFullContent = c.FormValue("scrape_full_content") == "true"
	f.ScrapeMaxAgeDays = scrapeMaxAge
	f.ImageMode = feed.ImageMode(c.FormValue("image_mode"))
	scrapeMethod := feed.ScrapeMethod(c.FormValue("scrape_method"))
	if scrapeMethod != feed.ScrapeMethodReadability && scrapeMethod != feed.ScrapeMethodSelector {
		scrapeMethod = feed.ScrapeMethodReadability
	}
	f.ScrapeMethod = scrapeMethod
	selectorType := feed.SelectorType(c.FormValue("scrape_selector_type"))
	if selectorType != feed.SelectorTypeCSS && selectorType != feed.SelectorTypeXPath {
		selectorType = feed.SelectorTypeCSS
	}
	f.ScrapeSelectorType = selectorType

	if ua := strings.TrimSpace(c.FormValue("user_agent")); ua != "" {
		if len(ua) > 500 {
			return echo.NewHTTPError(http.StatusBadRequest, "user agent must be 500 characters or fewer")
		}
		f.UserAgent = &ua
	} else {
		f.UserAgent = nil
	}
	if sel := strings.TrimSpace(c.FormValue("scrape_selector")); sel != "" {
		if len(sel) > 1000 {
			return echo.NewHTTPError(http.StatusBadRequest, "scrape selector must be 1000 characters or fewer")
		}
		f.ScrapeSelector = &sel
	} else {
		f.ScrapeSelector = nil
	}
	if ck := strings.TrimSpace(c.FormValue("scrape_cookies")); ck != "" {
		if len(ck) > 8192 {
			return echo.NewHTTPError(http.StatusBadRequest, "cookie header must be 8192 characters or fewer")
		}
		f.ScrapeCookies = &ck
	} else {
		f.ScrapeCookies = nil
	}
	if raw := strings.TrimSpace(c.FormValue("scrape_strip_selectors")); raw != "" {
		if len(raw) > 4096 {
			return echo.NewHTTPError(http.StatusBadRequest, "content strip selectors must be 4096 characters or fewer")
		}
		f.ScrapeStripSelectors = &raw
	} else {
		f.ScrapeStripSelectors = nil
	}
	if raw := strings.TrimSpace(c.FormValue("scrape_page_strip_selectors")); raw != "" {
		if len(raw) > 4096 {
			return echo.NewHTTPError(http.StatusBadRequest, "page strip selectors must be 4096 characters or fewer")
		}
		f.ScrapePageStripSelectors = &raw
	} else {
		f.ScrapePageStripSelectors = nil
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

func (h *FeedHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	h.poller.StopFeed(id)
	if err := h.feeds.Delete(c.Request().Context(), id); err != nil {
		slog.Error("delete feed", "feed_id", id, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete feed")
	}
	return redirect(c, "/feeds")
}

func (h *FeedHandler) Refresh(c echo.Context) error {
	h.poller.ForceRefresh(c.Param("id"))
	return redirect(c, "/feeds")
}

func (h *FeedHandler) Preview(c echo.Context) error {
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

	return pages.FeedPreview(csrfToken(c), f, arts, filterRules, showRaw).Render(ctx, c.Response().Writer)
}

// Scrape triggers a background re-scrape of all articles for this feed.
func (h *FeedHandler) Scrape(c echo.Context) error {
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
func (h *FeedHandler) PurgeScrape(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if err := h.articles.PurgeScrapeContent(ctx, id); err != nil {
		slog.Error("purge scrape content", "feed_id", id, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to purge content")
	}
	return redirect(c, "/feeds/"+id+"/edit")
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
