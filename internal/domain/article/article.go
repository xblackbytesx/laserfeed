package article

import (
	"context"
	"time"
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
}

type Repository interface {
	Upsert(ctx context.Context, a *Article) error
	ListByFeedID(ctx context.Context, feedID string, includeFiltered bool, limit, offset int) ([]*Article, error)
	ListByFeedIDs(ctx context.Context, feedIDs []string, limit, offset int) ([]*Article, error)
	ListRecent(ctx context.Context, limit, offset int) ([]*Article, error)
	CountByFeedID(ctx context.Context, feedID string) (int, error)
	DeleteOldest(ctx context.Context, feedID string, keepCount int) error
}
