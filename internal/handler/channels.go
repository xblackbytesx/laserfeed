package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/web/templates/pages"
)

type ChannelHandler struct {
	channels channel.Repository
	feeds    feed.Repository
}

func NewChannelHandler(channels channel.Repository, feeds feed.Repository) *ChannelHandler {
	return &ChannelHandler{channels: channels, feeds: feeds}
}

func (h *ChannelHandler) List(c echo.Context) error {
	chans, err := h.channels.List(c.Request().Context())
	if err != nil {
		chans = nil
	}
	return pages.ChannelList(csrfToken(c), chans).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ChannelHandler) Create(c echo.Context) error {
	ch := &channel.Channel{
		Name:        c.FormValue("name"),
		Slug:        c.FormValue("slug"),
		Description: c.FormValue("description"),
	}
	if _, err := h.channels.Create(c.Request().Context(), ch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return redirect(c, "/channels")
}

func (h *ChannelHandler) Edit(c echo.Context) error {
	ctx := c.Request().Context()
	ch, err := h.channels.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}
	channelFeeds, _ := h.channels.ListFeeds(ctx, ch.ID)
	allFeeds, _ := h.feeds.List(ctx)
	return pages.ChannelEdit(csrfToken(c), ch, channelFeeds, allFeeds).Render(ctx, c.Response().Writer)
}

func (h *ChannelHandler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	ch, err := h.channels.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}
	ch.Name = c.FormValue("name")
	ch.Slug = c.FormValue("slug")
	ch.Description = c.FormValue("description")
	if _, err := h.channels.Update(ctx, ch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return redirect(c, "/channels/"+ch.ID+"/edit")
}

func (h *ChannelHandler) Delete(c echo.Context) error {
	if err := h.channels.Delete(c.Request().Context(), c.Param("id")); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return redirect(c, "/channels")
}

func (h *ChannelHandler) AddFeed(c echo.Context) error {
	ctx := c.Request().Context()
	channelID := c.Param("id")
	feedID := c.FormValue("feed_id")

	if err := h.channels.AddFeed(ctx, channelID, feedID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	channelFeeds, _ := h.channels.ListFeeds(ctx, channelID)
	return pages.ChannelFeedsList(csrfToken(c), channelID, channelFeeds).Render(ctx, c.Response().Writer)
}

func (h *ChannelHandler) RemoveFeed(c echo.Context) error {
	ctx := c.Request().Context()
	channelID := c.Param("id")
	feedID := c.Param("fid")

	if err := h.channels.RemoveFeed(ctx, channelID, feedID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	channelFeeds, _ := h.channels.ListFeeds(ctx, channelID)
	return pages.ChannelFeedsList(csrfToken(c), channelID, channelFeeds).Render(ctx, c.Response().Writer)
}
