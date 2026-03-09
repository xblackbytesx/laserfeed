package channel

import (
	"context"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

type Channel struct {
	ID          string
	Name        string
	Slug        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Repository interface {
	Create(ctx context.Context, c *Channel) (*Channel, error)
	GetByID(ctx context.Context, id string) (*Channel, error)
	GetBySlug(ctx context.Context, slug string) (*Channel, error)
	List(ctx context.Context) ([]*Channel, error)
	Update(ctx context.Context, c *Channel) (*Channel, error)
	Delete(ctx context.Context, id string) error
	AddFeed(ctx context.Context, channelID, feedID string) error
	RemoveFeed(ctx context.Context, channelID, feedID string) error
	ListFeeds(ctx context.Context, channelID string) ([]*feed.Feed, error)
}
