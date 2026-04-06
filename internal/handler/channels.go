package handler

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/web/templates/pages"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type ChannelHandler struct {
	channels channel.Repository
	feeds    feed.Repository
}

func NewChannelHandler(channels channel.Repository, feeds feed.Repository) *ChannelHandler {
	return &ChannelHandler{channels: channels, feeds: feeds}
}

func (h *ChannelHandler) List(c *echo.Context) error {
	chans, err := h.channels.List(c.Request().Context())
	if err != nil {
		slog.Error("list channels", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channels")
	}
	return pages.ChannelList(csrfToken(c), chans).Render(c.Request().Context(), c.Response())
}

func (h *ChannelHandler) Create(c *echo.Context) error {
	name, slug, desc, err := validateChannelFields(c)
	if err != nil {
		return err
	}
	ch := &channel.Channel{Name: name, Slug: slug, Description: desc}
	if _, err := h.channels.Create(c.Request().Context(), ch); err != nil {
		slog.Error("create channel", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create channel")
	}
	return redirect(c, "/channels")
}

func (h *ChannelHandler) Edit(c *echo.Context) error {
	ctx := c.Request().Context()
	ch, err := h.channels.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}
	channelFeeds, err := h.channels.ListFeeds(ctx, ch.ID)
	if err != nil {
		slog.Error("list channel feeds", "channel_id", ch.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
	}
	allFeeds, err := h.feeds.List(ctx)
	if err != nil {
		slog.Error("list all feeds for channel edit", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feeds")
	}
	return pages.ChannelEdit(csrfToken(c), ch, channelFeeds, allFeeds).Render(ctx, c.Response())
}

func (h *ChannelHandler) Update(c *echo.Context) error {
	ctx := c.Request().Context()
	ch, err := h.channels.GetByID(ctx, c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}
	name, slug, desc, err := validateChannelFields(c)
	if err != nil {
		return err
	}
	ch.Name = name
	ch.Slug = slug
	ch.Description = desc
	if _, err := h.channels.Update(ctx, ch); err != nil {
		slog.Error("update channel", "channel_id", ch.ID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update channel")
	}
	return redirect(c, "/channels/"+ch.ID+"/edit")
}

func (h *ChannelHandler) Delete(c *echo.Context) error {
	if err := h.channels.Delete(c.Request().Context(), c.Param("id")); err != nil {
		slog.Error("delete channel", "channel_id", c.Param("id"), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete channel")
	}
	return redirect(c, "/channels")
}

func (h *ChannelHandler) AddFeed(c *echo.Context) error {
	ctx := c.Request().Context()
	channelID := c.Param("id")
	feedID := strings.TrimSpace(c.FormValue("feed_id"))
	if feedID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "feed_id is required")
	}

	if err := h.channels.AddFeed(ctx, channelID, feedID); err != nil {
		slog.Error("add feed to channel", "channel_id", channelID, "feed_id", feedID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add feed")
	}

	channelFeeds, err := h.channels.ListFeeds(ctx, channelID)
	if err != nil {
		slog.Error("list channel feeds after add", "channel_id", channelID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
	}
	return pages.ChannelFeedsList(csrfToken(c), channelID, channelFeeds).Render(ctx, c.Response())
}

func (h *ChannelHandler) RemoveFeed(c *echo.Context) error {
	ctx := c.Request().Context()
	channelID := c.Param("id")
	feedID := c.Param("fid")

	if err := h.channels.RemoveFeed(ctx, channelID, feedID); err != nil {
		slog.Error("remove feed from channel", "channel_id", channelID, "feed_id", feedID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove feed")
	}

	channelFeeds, err := h.channels.ListFeeds(ctx, channelID)
	if err != nil {
		slog.Error("list channel feeds after remove", "channel_id", channelID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
	}
	return pages.ChannelFeedsList(csrfToken(c), channelID, channelFeeds).Render(ctx, c.Response())
}

func validateChannelFields(c *echo.Context) (name, slug, desc string, err error) {
	name = strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if len(name) > 255 {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "name must be 255 characters or fewer")
	}

	slug = strings.TrimSpace(c.FormValue("slug"))
	if slug == "" {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "slug is required")
	}
	if len(slug) > 100 {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "slug must be 100 characters or fewer")
	}
	if !slugRe.MatchString(slug) {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "slug must contain only lowercase letters, numbers, and hyphens")
	}

	desc = strings.TrimSpace(c.FormValue("description"))
	if len(desc) > 1000 {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "description must be 1000 characters or fewer")
	}

	return name, slug, desc, nil
}
