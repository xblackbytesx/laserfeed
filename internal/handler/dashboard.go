package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/web/templates/pages"
)

type DashboardHandler struct {
	articles article.Repository
	channels channel.Repository
}

func NewDashboardHandler(articles article.Repository, channels channel.Repository) *DashboardHandler {
	return &DashboardHandler{articles: articles, channels: channels}
}

func (h *DashboardHandler) Get(c echo.Context) error {
	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * 20

	channelSlug := c.QueryParam("channel")

	chans, err := h.channels.List(ctx)
	if err != nil {
		slog.Error("list channels for dashboard", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channels")
	}

	var arts []*article.Article
	var activeChan *channel.Channel

	if channelSlug != "" {
		for _, ch := range chans {
			if ch.Slug == channelSlug {
				activeChan = ch
				break
			}
		}
	}

	if activeChan != nil {
		feeds, err := h.channels.ListFeeds(ctx, activeChan.ID)
		if err != nil {
			slog.Error("list channel feeds for dashboard", "channel_id", activeChan.ID, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load channel feeds")
		}
		feedIDs := make([]string, len(feeds))
		for i, f := range feeds {
			feedIDs[i] = f.ID
		}
		arts, err = h.articles.ListByFeedIDs(ctx, feedIDs, 20, offset)
		if err != nil {
			slog.Error("list articles by feed ids", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load articles")
		}
	} else {
		arts, err = h.articles.ListRecent(ctx, 20, offset)
		if err != nil {
			slog.Error("list recent articles", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load articles")
		}
	}

	return pages.Dashboard(csrfToken(c), arts, chans, activeChan, page).Render(ctx, c.Response().Writer)
}
