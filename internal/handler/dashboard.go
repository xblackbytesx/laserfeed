package handler

import (
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
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * 20

	arts, err := h.articles.ListRecent(c.Request().Context(), 20, offset)
	if err != nil {
		arts = nil
	}

	chans, err := h.channels.List(c.Request().Context())
	if err != nil {
		chans = nil
	}

	return pages.Dashboard(csrfToken(c), arts, chans, page).Render(c.Request().Context(), c.Response().Writer)
}

