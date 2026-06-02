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
	ImageModeBuiltin     ImageMode = "builtin"
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

// Valid reports whether m is a recognised image mode.
func (m ImageMode) Valid() bool {
	switch m {
	case ImageModeNone, ImageModePlaceholder, ImageModeRandom, ImageModeBuiltin:
		return true
	default:
		return false
	}
}

// NormalizeImageMode coerces a raw form/import value into a valid ImageMode.
// The retired "extract" mode maps to None; anything else unrecognised falls
// back to Random.
func NormalizeImageMode(s string) ImageMode {
	m := ImageMode(s)
	if m == "extract" {
		return ImageModeNone
	}
	if !m.Valid() {
		return ImageModeRandom
	}
	return m
}

// NormalizeScrapeMethod coerces a raw value into a valid ScrapeMethod,
// defaulting to Readability.
func NormalizeScrapeMethod(s string) ScrapeMethod {
	if m := ScrapeMethod(s); m == ScrapeMethodReadability || m == ScrapeMethodSelector {
		return m
	}
	return ScrapeMethodReadability
}

// NormalizeSelectorType coerces a raw value into a valid SelectorType,
// defaulting to CSS.
func NormalizeSelectorType(s string) SelectorType {
	if t := SelectorType(s); t == SelectorTypeCSS || t == SelectorTypeXPath {
		return t
	}
	return SelectorTypeCSS
}

type Feed struct {
	ID                       string
	Name                     string
	URL                      string
	Enabled                  bool
	PollIntervalSeconds      int
	UserAgent                *string
	ScrapeFullContent        bool
	ScrapeMethod             ScrapeMethod
	ScrapeSelector           *string
	ScrapeSelectorType       SelectorType
	ScrapeMaxAgeDays         int     // 0 = keep forever
	ScrapeCookies            *string // raw Cookie header value, e.g. "foo=bar; baz=qux"
	ScrapeStripSelectors     *string // newline-separated CSS selectors to remove from scraped content
	ScrapePageStripSelectors *string // newline-separated CSS selectors to remove from the full page before extraction
	RetentionMaxItems        int     // 0 = use global default, >0 = keep at most N articles
	RetentionMaxHours        int     // 0 = disabled, >0 = delete articles older than N hours
	ImageMode                ImageMode
	PlaceholderImageURL      *string
	LastPolledAt             *time.Time
	LastError                *string
	PollETag                 string // HTTP ETag from the last successful poll (conditional GET)
	PollLastModified         string // HTTP Last-Modified from the last successful poll
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type Repository interface {
	Create(ctx context.Context, f *Feed) (*Feed, error)
	GetByID(ctx context.Context, id string) (*Feed, error)
	List(ctx context.Context) ([]*Feed, error)
	ListByIDs(ctx context.Context, ids []string) ([]*Feed, error)
	ListEnabled(ctx context.Context) ([]*Feed, error)
	Update(ctx context.Context, f *Feed) (*Feed, error)
	Delete(ctx context.Context, id string) error
	UpdatePollStatus(ctx context.Context, id string, lastPolledAt time.Time, lastError *string) error
	// UpdatePollValidators stores the HTTP cache validators from a successful poll.
	UpdatePollValidators(ctx context.Context, id, etag, lastModified string) error
}
