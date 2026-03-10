package middleware

import "github.com/labstack/echo/v4"

// pageTitles maps Echo route patterns to human-readable page titles.
// Only GET routes that render a full page are listed; redirects and
// HTMX partial-swap endpoints don't need a title.
var pageTitles = map[string]string{
	"/":                  "Dashboard",
	"/feeds":             "Feed Pool",
	"/feeds/:id/edit":    "Edit Feed",
	"/feeds/:id/preview": "Feed Preview",
	"/feeds/:id/rules":   "Filter Rules",
	"/channels":          "Channels",
	"/channels/:id/edit": "Edit Channel",
	"/settings":          "Settings",
}

// HXTitle sets the HX-Title response header on HTMX requests so the
// browser tab title stays correct during boost-powered navigation.
func HXTitle() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Header.Get("HX-Request") == "true" {
				if title, ok := pageTitles[c.Path()]; ok {
					c.Response().Header().Set("HX-Title", title+" — LaserFeed")
				}
			}
			return next(c)
		}
	}
}
