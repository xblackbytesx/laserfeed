package settings

import (
	"context"
)

type Settings struct {
	UserAgent           string
	PollIntervalSeconds int
	ImageMode           string
	PlaceholderImageURL string
	BuiltinPlaceholder  string // filename of selected built-in SVG, e.g. "laserfeed-placeholder.svg"
	MaxArticlesPerFeed  int
}

type Repository interface {
	Get(ctx context.Context) (*Settings, error)
	// SetAll persists all key/value pairs in a single round-trip.
	SetAll(ctx context.Context, pairs map[string]string) error
}
