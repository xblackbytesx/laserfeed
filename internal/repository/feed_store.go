package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

type FeedStore struct {
	db *pgxpool.Pool
}

func NewFeedStore(db *pgxpool.Pool) *FeedStore {
	return &FeedStore{db: db}
}

const feedCols = `id, name, url, enabled, poll_interval_seconds, user_agent,
	scrape_full_content, scrape_method, scrape_selector, scrape_selector_type, scrape_max_age_days, scrape_cookies,
	scrape_strip_selectors, scrape_page_strip_selectors,
	image_mode, placeholder_image_url, last_polled_at, last_error, created_at, updated_at`

func scanFeed(row interface{ Scan(...any) error }) (*feed.Feed, error) {
	f := &feed.Feed{}
	var scrapeSelector, userAgent, placeholderImageURL, lastError, scrapeCookies, scrapeStripSelectors, scrapePageStripSelectors *string
	var lastPolledAt *time.Time
	var imageModeStr, scrapeMethodStr, selectorTypeStr string
	err := row.Scan(
		&f.ID, &f.Name, &f.URL, &f.Enabled, &f.PollIntervalSeconds, &userAgent,
		&f.ScrapeFullContent, &scrapeMethodStr, &scrapeSelector, &selectorTypeStr, &f.ScrapeMaxAgeDays, &scrapeCookies,
		&scrapeStripSelectors, &scrapePageStripSelectors,
		&imageModeStr, &placeholderImageURL, &lastPolledAt, &lastError,
		&f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	f.UserAgent = userAgent
	f.ScrapeMethod = feed.ScrapeMethod(scrapeMethodStr)
	f.ScrapeSelector = scrapeSelector
	f.ScrapeSelectorType = feed.SelectorType(selectorTypeStr)
	f.ScrapeCookies = scrapeCookies
	f.ScrapeStripSelectors = scrapeStripSelectors
	f.ScrapePageStripSelectors = scrapePageStripSelectors
	f.ImageMode = feed.ImageMode(imageModeStr)
	f.PlaceholderImageURL = placeholderImageURL
	f.LastPolledAt = lastPolledAt
	f.LastError = lastError
	return f, nil
}

func (s *FeedStore) Create(ctx context.Context, f *feed.Feed) (*feed.Feed, error) {
	row := s.db.QueryRow(ctx,
		`INSERT INTO feeds (name, url, enabled, poll_interval_seconds, user_agent,
			scrape_full_content, scrape_method, scrape_selector, scrape_selector_type, scrape_max_age_days, scrape_cookies,
			scrape_strip_selectors, scrape_page_strip_selectors,
			image_mode, placeholder_image_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+feedCols,
		f.Name, f.URL, f.Enabled, f.PollIntervalSeconds, f.UserAgent,
		f.ScrapeFullContent, string(f.ScrapeMethod), f.ScrapeSelector, string(f.ScrapeSelectorType), f.ScrapeMaxAgeDays, f.ScrapeCookies,
		f.ScrapeStripSelectors, f.ScrapePageStripSelectors,
		string(f.ImageMode), f.PlaceholderImageURL,
	)
	created, err := scanFeed(row)
	if err != nil {
		return nil, fmt.Errorf("create feed: %w", err)
	}
	return created, nil
}

func (s *FeedStore) GetByID(ctx context.Context, id string) (*feed.Feed, error) {
	row := s.db.QueryRow(ctx, `SELECT `+feedCols+` FROM feeds WHERE id=$1`, id)
	f, err := scanFeed(row)
	if err != nil {
		return nil, fmt.Errorf("get feed: %w", err)
	}
	return f, nil
}

func (s *FeedStore) List(ctx context.Context) ([]*feed.Feed, error) {
	rows, err := s.db.Query(ctx, `SELECT `+feedCols+` FROM feeds ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list feeds: %w", err)
	}
	defer rows.Close()
	var feeds []*feed.Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *FeedStore) ListEnabled(ctx context.Context) ([]*feed.Feed, error) {
	rows, err := s.db.Query(ctx, `SELECT `+feedCols+` FROM feeds WHERE enabled=true ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list enabled feeds: %w", err)
	}
	defer rows.Close()
	var feeds []*feed.Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *FeedStore) Update(ctx context.Context, f *feed.Feed) (*feed.Feed, error) {
	row := s.db.QueryRow(ctx,
		`UPDATE feeds SET name=$1, url=$2, enabled=$3, poll_interval_seconds=$4,
			user_agent=$5, scrape_full_content=$6, scrape_method=$7, scrape_selector=$8,
			scrape_selector_type=$9, scrape_max_age_days=$10, scrape_cookies=$11,
			scrape_strip_selectors=$12, scrape_page_strip_selectors=$13,
			image_mode=$14, placeholder_image_url=$15,
			updated_at=NOW()
		WHERE id=$16
		RETURNING `+feedCols,
		f.Name, f.URL, f.Enabled, f.PollIntervalSeconds, f.UserAgent,
		f.ScrapeFullContent, string(f.ScrapeMethod), f.ScrapeSelector, string(f.ScrapeSelectorType), f.ScrapeMaxAgeDays, f.ScrapeCookies,
		f.ScrapeStripSelectors, f.ScrapePageStripSelectors,
		string(f.ImageMode), f.PlaceholderImageURL, f.ID,
	)
	updated, err := scanFeed(row)
	if err != nil {
		return nil, fmt.Errorf("update feed: %w", err)
	}
	return updated, nil
}

func (s *FeedStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM feeds WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	return nil
}

func (s *FeedStore) UpdatePollStatus(ctx context.Context, id string, lastPolledAt time.Time, lastError *string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE feeds SET last_polled_at=$1, last_error=$2, updated_at=NOW() WHERE id=$3`,
		lastPolledAt, lastError, id,
	)
	if err != nil {
		return fmt.Errorf("update poll status: %w", err)
	}
	return nil
}
