package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/csrf"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/laserfeed/laserfeed/internal/config"
	appdb "github.com/laserfeed/laserfeed/internal/db"
	"github.com/laserfeed/laserfeed/internal/handler"
	appmiddleware "github.com/laserfeed/laserfeed/internal/middleware"
	"github.com/laserfeed/laserfeed/internal/poller"
	"github.com/laserfeed/laserfeed/internal/repository"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if err := appdb.RunMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("run migrations", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := appdb.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	feedStore := repository.NewFeedStore(pool)
	articleStore := repository.NewArticleStore(pool)
	filterRuleStore := repository.NewFilterRuleStore(pool)
	settingsStore := repository.NewSettingsStore(pool)
	channelStore := repository.NewChannelStore(pool)

	pollerManager := poller.NewManager(ctx, poller.Stores{
		Feeds:       feedStore,
		Articles:    articleStore,
		FilterRules: filterRuleStore,
		Settings:    settingsStore,
	})
	go pollerManager.Start()

	dashHandler := handler.NewDashboardHandler(articleStore, channelStore)
	feedHandler := handler.NewFeedHandler(feedStore, articleStore, filterRuleStore, pollerManager)
	rulesHandler := handler.NewRulesHandler(feedStore, filterRuleStore)
	channelHandler := handler.NewChannelHandler(channelStore, feedStore)
	settingsHandler := handler.NewSettingsHandler(settingsStore)
	feedOutHandler := handler.NewFeedOutHandler(channelStore, articleStore, cfg.AppBaseURL)

	e := echo.New()
	e.HideBanner = true
	e.Use(echomiddleware.Recover())
	e.Use(appmiddleware.Logger())
	e.Use(appmiddleware.HXTitle())

	// Security headers on every response
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			return next(c)
		}
	})

	csrfMiddleware := csrf.Protect(
		[]byte(cfg.CSRFAuthKey),
		csrf.Secure(cfg.SecureCookies),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.CookieName("csrf"),
	)
	e.Use(echo.WrapMiddleware(csrfMiddleware))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("csrf", csrf.Token(c.Request()))
			return next(c)
		}
	})

	e.Static("/static", "web/static")

	// Health check (no CSRF needed)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Dashboard
	e.GET("/", dashHandler.Get)

	// Feed pool
	e.GET("/feeds", feedHandler.List)
	e.POST("/feeds", feedHandler.Create)
	e.GET("/feeds/:id/edit", feedHandler.Edit)
	e.POST("/feeds/:id/update", feedHandler.Update)
	e.POST("/feeds/:id/delete", feedHandler.Delete)
	e.POST("/feeds/:id/refresh", feedHandler.Refresh)
	e.GET("/feeds/:id/preview", feedHandler.Preview)
	e.POST("/feeds/:id/scrape", feedHandler.Scrape)
	e.POST("/feeds/:id/scrape/purge", feedHandler.PurgeScrape)

	// Filter rules
	e.GET("/feeds/:id/rules", rulesHandler.List)
	e.POST("/feeds/:id/rules", rulesHandler.Create)
	e.POST("/feeds/:id/rules/:rid/delete", rulesHandler.Delete)

	// Channels
	e.GET("/channels", channelHandler.List)
	e.POST("/channels", channelHandler.Create)
	e.GET("/channels/:id/edit", channelHandler.Edit)
	e.POST("/channels/:id/update", channelHandler.Update)
	e.POST("/channels/:id/delete", channelHandler.Delete)
	e.POST("/channels/:id/feeds", channelHandler.AddFeed)
	e.POST("/channels/:id/feeds/:fid/remove", channelHandler.RemoveFeed)

	// Feed outputs (no CSRF needed for RSS consumers)
	e.GET("/channels/:slug/feed.rss", feedOutHandler.ChannelFeed)
	e.GET("/feed.rss", feedOutHandler.AllFeed)

	// Settings
	e.GET("/settings", settingsHandler.Get)
	e.POST("/settings", settingsHandler.Post)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		<-sig
		slog.Info("shutting down...")
		cancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = e.Shutdown(shutCtx)
	}()

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr, "secure_cookies", cfg.SecureCookies)
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
	}
}
