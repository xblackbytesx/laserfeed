package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

func Logger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)
			status := http.StatusOK
			if he, ok := err.(*echo.HTTPError); ok {
				status = he.Code
			} else if err != nil {
				status = http.StatusInternalServerError
			}
			slog.Info("request",
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"status", status,
				"duration", time.Since(start),
			)
			return err
		}
	}
}
