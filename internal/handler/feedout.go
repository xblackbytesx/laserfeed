package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/feedout"
)

type FeedOutHandler struct {
	channels   channel.Repository
	articles   article.Repository
	appBaseURL string
}

func NewFeedOutHandler(channels channel.Repository, articles article.Repository, appBaseURL string) *FeedOutHandler {
	return &FeedOutHandler{channels: channels, articles: articles, appBaseURL: appBaseURL}
}

func (h *FeedOutHandler) ChannelFeed(c echo.Context) error {
	ctx := c.Request().Context()
	slug := c.Param("slug")

	ch, err := h.channels.GetBySlug(ctx, slug)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}

	channelFeeds, err := h.channels.ListFeeds(ctx, ch.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	feedIDs := make([]string, len(channelFeeds))
	for i, f := range channelFeeds {
		feedIDs[i] = f.ID
	}

	arts, err := h.articles.ListByFeedIDs(ctx, feedIDs, 100, 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	atomBytes, err := feedout.GenerateAtom(ch, arts, h.appBaseURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.Blob(http.StatusOK, "application/atom+xml; charset=utf-8", atomBytes)
}

func (h *FeedOutHandler) AllFeed(c echo.Context) error {
	ctx := c.Request().Context()

	arts, err := h.articles.ListRecent(ctx, 100, 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Synthetic channel for the all-feeds output
	allCh := &channel.Channel{
		Name: "LaserFeed — All",
		Slug: "all",
	}

	atomBytes, err := feedout.GenerateAtom(allCh, arts, h.appBaseURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.Blob(http.StatusOK, "application/atom+xml; charset=utf-8", atomBytes)
}
