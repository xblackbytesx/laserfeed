package handler

import (
	"github.com/gorilla/csrf"
	"github.com/labstack/echo/v5"
)

func csrfToken(c *echo.Context) string {
	tok, _ := c.Get("csrf").(string)
	if tok == "" {
		tok = csrf.Token(c.Request())
	}
	return tok
}

func redirect(c *echo.Context, path string) error {
	return c.Redirect(303, path)
}
