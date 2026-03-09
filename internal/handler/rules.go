package handler

import (
	"net/http"

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
		rules = nil
	}

	return pages.RulesPage(csrfToken(c), f, rules).Render(ctx, c.Response().Writer)
}

func (h *RulesHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	feedID := c.Param("id")

	r := &filterrule.FilterRule{
		FeedID:       feedID,
		RuleType:     filterrule.RuleType(c.FormValue("rule_type")),
		MatchField:   filterrule.MatchField(c.FormValue("match_field")),
		MatchPattern: c.FormValue("match_pattern"),
	}

	if _, err := h.rules.Create(ctx, r); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	rules, _ := h.rules.ListByFeedID(ctx, feedID)
	return pages.RulesList(csrfToken(c), feedID, rules).Render(ctx, c.Response().Writer)
}

func (h *RulesHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	feedID := c.Param("id")
	ruleID := c.Param("rid")

	if err := h.rules.Delete(ctx, ruleID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	rules, _ := h.rules.ListByFeedID(ctx, feedID)
	return pages.RulesList(csrfToken(c), feedID, rules).Render(ctx, c.Response().Writer)
}
