package handler

import (
	"log/slog"
	"net/http"
	"strconv"

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
		slog.Error("get settings", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load settings")
	}
	return pages.SettingsPage(csrfToken(c), s).Render(c.Request().Context(), c.Response().Writer)
}

func (h *SettingsHandler) Post(c echo.Context) error {
	ctx := c.Request().Context()

	pollInterval, err := strconv.Atoi(c.FormValue("poll_interval_seconds"))
	if err != nil || pollInterval < 60 {
		return echo.NewHTTPError(http.StatusBadRequest, "poll interval must be a number and at least 60 seconds")
	}

	maxArticles, err := strconv.Atoi(c.FormValue("max_articles_per_feed"))
	if err != nil || maxArticles < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "max articles per feed must be a positive number")
	}
	if maxArticles > 100000 {
		return echo.NewHTTPError(http.StatusBadRequest, "max articles per feed must be 100000 or fewer")
	}

	imageMode := c.FormValue("image_mode")
	switch imageMode {
	case "none", "extract", "placeholder", "random":
		// valid
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid image mode")
	}

	ua := c.FormValue("user_agent")
	if len(ua) > 500 {
		return echo.NewHTTPError(http.StatusBadRequest, "user agent must be 500 characters or fewer")
	}

	ph := c.FormValue("placeholder_image_url")
	if ph != "" {
		if err := validateFeedURL(ph); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid placeholder image URL")
		}
	}

	pairs := map[string]string{
		"user_agent":            ua,
		"poll_interval_seconds": strconv.Itoa(pollInterval),
		"image_mode":            imageMode,
		"placeholder_image_url": ph,
		"max_articles_per_feed": strconv.Itoa(maxArticles),
	}
	if err := h.settings.SetAll(ctx, pairs); err != nil {
		slog.Error("save settings", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save settings")
	}
	return redirect(c, "/settings")
}
