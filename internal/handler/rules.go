package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
	"github.com/laserfeed/laserfeed/web/templates/pages"
)

type RulesHandler struct {
	feeds feed.Repository
	rules filterrule.Repository
}

func NewRulesHandler(feeds feed.Repository, rules filterrule.Repository) *RulesHandler {
	return &RulesHandler{feeds: feeds, rules: rules}
}

func (h *RulesHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	feedID := c.Param("id")

	f, err := h.feeds.GetByID(ctx, feedID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "feed not found")
	}

	rules, err := h.rules.ListByFeedID(ctx, feedID)
	if err != nil {
		slog.Error("list filter rules", "feed_id", feedID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load filter rules")
	}

	return pages.RulesPage(csrfToken(c), f, rules).Render(ctx, c.Response().Writer)
}

func (h *RulesHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	feedID := c.Param("id")

	ruleType := filterrule.RuleType(c.FormValue("rule_type"))
	if ruleType != filterrule.RuleTypeWhitelist && ruleType != filterrule.RuleTypeBlacklist {
		return echo.NewHTTPError(http.StatusBadRequest, "rule_type must be 'whitelist' or 'blacklist'")
	}

	matchField := filterrule.MatchField(c.FormValue("match_field"))
	switch matchField {
	case filterrule.MatchFieldTitle, filterrule.MatchFieldURL,
		filterrule.MatchFieldContent, filterrule.MatchFieldDescription:
		// valid
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "match_field must be one of: title, url, content, description")
	}

	pattern := strings.TrimSpace(c.FormValue("match_pattern"))
	if pattern == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "match_pattern is required")
	}
	if len(pattern) > 500 {
		return echo.NewHTTPError(http.StatusBadRequest, "match_pattern must be 500 characters or fewer")
	}

	r := &filterrule.FilterRule{
		FeedID:       feedID,
		RuleType:     ruleType,
		MatchField:   matchField,
		MatchPattern: pattern,
	}

	if _, err := h.rules.Create(ctx, r); err != nil {
		slog.Error("create filter rule", "feed_id", feedID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create rule")
	}

	rules, err := h.rules.ListByFeedID(ctx, feedID)
	if err != nil {
		slog.Error("list filter rules after create", "feed_id", feedID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load filter rules")
	}
	return pages.RulesList(csrfToken(c), feedID, rules).Render(ctx, c.Response().Writer)
}

func (h *RulesHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	feedID := c.Param("id")
	ruleID := c.Param("rid")

	if err := h.rules.Delete(ctx, feedID, ruleID); err != nil {
		slog.Error("delete filter rule", "rule_id", ruleID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete rule")
	}

	rules, err := h.rules.ListByFeedID(ctx, feedID)
	if err != nil {
		slog.Error("list filter rules after delete", "feed_id", feedID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load filter rules")
	}
	return pages.RulesList(csrfToken(c), feedID, rules).Render(ctx, c.Response().Writer)
}
