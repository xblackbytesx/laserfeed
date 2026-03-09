package poller

import (
	"context"
	"log/slog"
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
