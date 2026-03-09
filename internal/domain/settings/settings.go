package settings

import (
	"context"
)

type Settings struct {
	UserAgent           string
	PollIntervalSeconds int
	ImageMode           string
	PlaceholderImageURL string
	MaxArticlesPerFeed  int
}

type Repository interface {
	Get(ctx context.Context) (*Settings, error)
	Set(ctx context.Context, key, value string) error
}
