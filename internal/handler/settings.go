package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/laserfeed/laserfeed/internal/domain/settings"
	"github.com/laserfeed/laserfeed/web/templates/pages"
)

type SettingsHandler struct {
	settings settings.Repository
}

func NewSettingsHandler(s settings.Repository) *SettingsHandler {
	return &SettingsHandler{settings: s}
}

func (h *SettingsHandler) Get(c echo.Context) error {
	s, err := h.settings.Get(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return pages.SettingsPage(csrfToken(c), s).Render(c.Request().Context(), c.Response().Writer)
}

func (h *SettingsHandler) Post(c echo.Context) error {
	ctx := c.Request().Context()
	pairs := map[string]string{
		"user_agent":            c.FormValue("user_agent"),
		"poll_interval_seconds": c.FormValue("poll_interval_seconds"),
		"image_mode":            c.FormValue("image_mode"),
		"placeholder_image_url": c.FormValue("placeholder_image_url"),
		"max_articles_per_feed": c.FormValue("max_articles_per_feed"),
	}
	for k, v := range pairs {
		if err := h.settings.Set(ctx, k, v); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	return redirect(c, "/settings")
}
