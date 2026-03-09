package handler

import (
	"net/http"
	"strconv"

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
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return pages.FeedList(csrfToken(c), feeds).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FeedHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()

	pollInterval, _ := strconv.Atoi(c.FormValue("poll_interval_seconds"))
	if pollInterval <= 0 {
		pollInterval = 3600
	}

	imageMode := feed.ImageMode(c.FormValue("image_mode"))
	if imageMode == "" {
		imageMode = feed.ImageModeExtract
	}

	f := &feed.Feed{
		Name:                c.FormValue("name"),
		URL:                 c.FormValue("url"),
		Enabled:             true,
		PollIntervalSeconds: pollInterval,
		ScrapeSelectorType:  feed.SelectorTypeCSS,
		ImageMode:           imageMode,
	}

	created, err := h.feeds.Create(ctx, f)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// StartFeed uses the app-level context stored in the manager — not the request context
	h.poller.StartFeed(created)
	h.poller.ForceRefresh(created.ID)

	return redirect(c, "/feeds")
}

func (h *FeedHandler) Edit(c echo.Context) error {
	f, err := h.feeds.GetByID(c.Request().Context(), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}
	return pages.FeedEdit(csrfToken(c), f).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FeedHandler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	f, err := h.feeds.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}

	pollInterval, _ := strconv.Atoi(c.FormValue("poll_interval_seconds"))
	if pollInterval <= 0 {
		pollInterval = 3600
	}

	f.Name = c.FormValue("name")
	f.URL = c.FormValue("url")
	f.Enabled = c.FormValue("enabled") == "true"
	f.PollIntervalSeconds = pollInterval
	f.ScrapeFullContent = c.FormValue("scrape_full_content") == "true"
	f.ImageMode = feed.ImageMode(c.FormValue("image_mode"))
	f.ScrapeSelectorType = feed.SelectorType(c.FormValue("scrape_selector_type"))

	if ua := c.FormValue("user_agent"); ua != "" {
		f.UserAgent = &ua
	} else {
		f.UserAgent = nil
	}
	if sel := c.FormValue("scrape_selector"); sel != "" {
		f.ScrapeSelector = &sel
	} else {
		f.ScrapeSelector = nil
	}
	if ph := c.FormValue("placeholder_image_url"); ph != "" {
		f.PlaceholderImageURL = &ph
	} else {
		f.PlaceholderImageURL = nil
	}

	updated, err := h.feeds.Update(ctx, f)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if updated.Enabled {
		h.poller.StartFeed(updated)
	} else {
		h.poller.StopFeed(updated.ID)
	}

	return redirect(c, "/feeds")
}

func (h *FeedHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	h.poller.StopFeed(id)
	if err := h.feeds.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
		arts = nil
	}

	filterRules, _ := h.rules.ListByFeedID(ctx, f.ID)

	return pages.FeedPreview(csrfToken(c), f, arts, filterRules, showRaw).Render(ctx, c.Response().Writer)
}
