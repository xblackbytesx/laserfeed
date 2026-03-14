package poller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/feed"
	"github.com/laserfeed/laserfeed/internal/scraper"
)

type feedPollState struct {
	cancel    context.CancelFunc
	refreshCh chan struct{}
}

type Manager struct {
	mu      sync.Mutex
	states  map[string]*feedPollState
	stores  Stores
	scraper *scraper.Scraper
	rootCtx context.Context
}

func NewManager(ctx context.Context, stores Stores) *Manager {
	return &Manager{
		rootCtx: ctx,
		states:  make(map[string]*feedPollState),
		stores:  stores,
		scraper: scraper.New(),
	}
}

func (m *Manager) Start() {
	feeds, err := m.stores.Feeds.ListEnabled(m.rootCtx)
	if err != nil {
		slog.Error("poller manager: list enabled feeds", "err", err)
		return
	}
	for _, f := range feeds {
		m.StartFeed(f)
	}
}

func (m *Manager) StartFeed(f *feed.Feed) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.states[f.ID]; exists {
		return
	}

	fctx, cancel := context.WithCancel(m.rootCtx)
	refreshCh := make(chan struct{}, 1)

	m.states[f.ID] = &feedPollState{cancel: cancel, refreshCh: refreshCh}

	go m.runFeedLoop(fctx, f.ID, refreshCh)
}

func (m *Manager) StopFeed(feedID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.states[feedID]; ok {
		state.cancel()
		delete(m.states, feedID)
	}
}

func (m *Manager) ForceRefresh(feedID string) {
	m.mu.Lock()
	state, ok := m.states[feedID]
	m.mu.Unlock()
	if ok {
		select {
		case state.refreshCh <- struct{}{}:
		default:
		}
	}
}

// ReScrapeArticles re-fetches content for all articles in a feed in the background.
func (m *Manager) ReScrapeArticles(feedID string) {
	go func() {
		ctx := m.rootCtx

		f, err := m.stores.Feeds.GetByID(ctx, feedID)
		if err != nil {
			slog.Error("rescrape: get feed", "feed_id", feedID, "err", err)
			return
		}
		if !f.ScrapeFullContent {
			return
		}

		globalSettings, err := m.stores.Settings.Get(ctx)
		if err != nil {
			slog.Error("rescrape: get settings", "err", err)
			return
		}

		sp := resolveScrapeParams(f, globalSettings.UserAgent)

		refs, err := m.stores.Articles.ListForReScrape(ctx, feedID)
		if err != nil {
			slog.Error("rescrape: list articles", "feed_id", feedID, "err", err)
			return
		}

		slog.Info("rescrape: starting", "feed_id", feedID, "articles", len(refs))
		success, failed := 0, 0
		for _, ref := range refs {
			select {
			case <-ctx.Done():
				slog.Info("rescrape: cancelled", "feed_id", feedID)
				return
			default:
			}

			scraped, err := m.scraper.ScrapeContent(ctx, ref.URL, sp.userAgent, sp.selector, sp.selectorType, sp.cookies, sp.stripSelectors)
			var errMsg string
			switch {
			case err != nil:
				errMsg = err.Error()
			case strings.TrimSpace(scraped) == "":
				if sp.selector != "" {
					errMsg = fmt.Sprintf("selector %q matched no content", sp.selector)
				} else {
					errMsg = "no content could be extracted from the page"
				}
			}
			if errMsg != "" {
				if updateErr := m.stores.Articles.UpdateScrapeResult(ctx, ref.ID, "", errMsg); updateErr != nil {
					slog.Error("rescrape: update failed status", "id", ref.ID, "err", updateErr)
				}
				failed++
			} else {
				if updateErr := m.stores.Articles.UpdateScrapeResult(ctx, ref.ID, scraped, ""); updateErr != nil {
					slog.Error("rescrape: update success status", "id", ref.ID, "err", updateErr)
				}
				success++
			}
		}
		slog.Info("rescrape: done", "feed_id", feedID, "success", success, "failed", failed)
	}()
}

func (m *Manager) runFeedLoop(ctx context.Context, feedID string, refreshCh chan struct{}) {
	pollOnce(ctx, feedID, m.stores, m.scraper)

	for {
		f, err := m.stores.Feeds.GetByID(ctx, feedID)
		if err != nil {
			slog.Error("poller loop: get feed", "feed_id", feedID, "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(60 * time.Second):
				continue
			}
		}

		interval := time.Duration(f.PollIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = time.Hour
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			pollOnce(ctx, feedID, m.stores, m.scraper)
		case <-refreshCh:
			timer.Stop()
			pollOnce(ctx, feedID, m.stores, m.scraper)
		}
	}
}
