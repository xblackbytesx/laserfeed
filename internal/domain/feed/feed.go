package feed

import (
	"context"
	"time"
)

type ImageMode string

const (
	ImageModeNone        ImageMode = "none"
	ImageModePlaceholder ImageMode = "placeholder"
	ImageModeRandom      ImageMode = "random"
)

type ScrapeMethod string

const (
	ScrapeMethodReadability ScrapeMethod = "readability"
	ScrapeMethodSelector    ScrapeMethod = "selector"
)

type SelectorType string

const (
	SelectorTypeCSS   SelectorType = "css"
	SelectorTypeXPath SelectorType = "xpath"
)

type Feed struct {
	ID                  string
	Name                string
	URL                 string
	Enabled             bool
	PollIntervalSeconds int
	UserAgent           *string
	ScrapeFullContent   bool
	ScrapeMethod        ScrapeMethod
	ScrapeSelector      *string
	ScrapeSelectorType  SelectorType
	ScrapeMaxAgeDays        int     // 0 = keep forever
	ScrapeCookies           *string // raw Cookie header value, e.g. "foo=bar; baz=qux"
	ScrapeStripSelectors    *string // newline-separated CSS selectors to remove from scraped content
	ScrapePageStripSelectors *string // newline-separated CSS selectors to remove from the full page before extraction
	ImageMode           ImageMode
	PlaceholderImageURL *string
	LastPolledAt        *time.Time
	LastError           *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Repository interface {
	Create(ctx context.Context, f *Feed) (*Feed, error)
	GetByID(ctx context.Context, id string) (*Feed, error)
	List(ctx context.Context) ([]*Feed, error)
	ListEnabled(ctx context.Context) ([]*Feed, error)
	Update(ctx context.Context, f *Feed) (*Feed, error)
	Delete(ctx context.Context, id string) error
	UpdatePollStatus(ctx context.Context, id string, lastPolledAt time.Time, lastError *string) error
}
