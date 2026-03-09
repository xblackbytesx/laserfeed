package article

import (
	"context"
	"time"
)

type ScrapeStatus string

const (
	ScrapeStatusNone    ScrapeStatus = "none"
	ScrapeStatusSuccess ScrapeStatus = "success"
	ScrapeStatusFailed  ScrapeStatus = "failed"
)

type Article struct {
	ID            string
	FeedID        string
	GUID          string
	Title         string
	URL           string
	Author        string
	Description   string
	Content       string
	ThumbnailURL  string
	PublishedAt   time.Time
	FetchedAt     time.Time
	IsFilteredOut bool
	CreatedAt     time.Time
	ScrapeStatus  ScrapeStatus
	ScrapeError   string
}

// ScrapeStats holds aggregate scrape result counts for a feed.
type ScrapeStats struct {
	Success int
	Failed  int
	None    int
}

// ArticleRef is a lightweight reference used for re-scrape operations.
type ArticleRef struct {
	ID  string
	URL string
}

type Repository interface {
	Upsert(ctx context.Context, a *Article) error
	ListByFeedID(ctx context.Context, feedID string, includeFiltered bool, limit, offset int) ([]*Article, error)
	ListByFeedIDs(ctx context.Context, feedIDs []string, limit, offset int) ([]*Article, error)
	ListRecent(ctx context.Context, limit, offset int) ([]*Article, error)
	DeleteOldest(ctx context.Context, feedID string, keepCount int) error

	// Scrape tracking
	UpdateScrapeResult(ctx context.Context, id, content, errMsg string) error
	GetScrapeStats(ctx context.Context, feedID string) (*ScrapeStats, error)
	GetScrapedGUIDs(ctx context.Context, feedID string) (map[string]bool, error)
	ListForReScrape(ctx context.Context, feedID string) ([]*ArticleRef, error)
	PurgeScrapeContent(ctx context.Context, feedID string) error
	PurgeOldScrapeContent(ctx context.Context, feedID string, maxAgeDays int) error
}
