package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

type ChannelStore struct {
	db *pgxpool.Pool
}

func NewChannelStore(db *pgxpool.Pool) *ChannelStore {
	return &ChannelStore{db: db}
}

const channelCols = `id, name, slug, description, created_at, updated_at`

func scanChannel(row interface{ Scan(...any) error }) (*channel.Channel, error) {
	c := &channel.Channel{}
	err := row.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *ChannelStore) Create(ctx context.Context, c *channel.Channel) (*channel.Channel, error) {
	row := s.db.QueryRow(ctx,
		`INSERT INTO channels (name, slug, description) VALUES ($1,$2,$3) RETURNING `+channelCols,
		c.Name, c.Slug, c.Description,
	)
	created, err := scanChannel(row)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return created, nil
}

func (s *ChannelStore) GetByID(ctx context.Context, id string) (*channel.Channel, error) {
	row := s.db.QueryRow(ctx, `SELECT `+channelCols+` FROM channels WHERE id=$1`, id)
	c, err := scanChannel(row)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return c, nil
}

func (s *ChannelStore) GetBySlug(ctx context.Context, slug string) (*channel.Channel, error) {
	row := s.db.QueryRow(ctx, `SELECT `+channelCols+` FROM channels WHERE slug=$1`, slug)
	c, err := scanChannel(row)
	if err != nil {
		return nil, fmt.Errorf("get channel by slug: %w", err)
	}
	return c, nil
}

func (s *ChannelStore) List(ctx context.Context) ([]*channel.Channel, error) {
	rows, err := s.db.Query(ctx, `SELECT `+channelCols+` FROM channels ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var channels []*channel.Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

func (s *ChannelStore) Update(ctx context.Context, c *channel.Channel) (*channel.Channel, error) {
	row := s.db.QueryRow(ctx,
		`UPDATE channels SET name=$1, slug=$2, description=$3, updated_at=NOW()
		WHERE id=$4 RETURNING `+channelCols,
		c.Name, c.Slug, c.Description, c.ID,
	)
	updated, err := scanChannel(row)
	if err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return updated, nil
}

func (s *ChannelStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM channels WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	return nil
}

func (s *ChannelStore) AddFeed(ctx context.Context, channelID, feedID string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO channel_feeds (channel_id, feed_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		channelID, feedID,
	)
	if err != nil {
		return fmt.Errorf("add feed to channel: %w", err)
	}
	return nil
}

func (s *ChannelStore) RemoveFeed(ctx context.Context, channelID, feedID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM channel_feeds WHERE channel_id=$1 AND feed_id=$2`,
		channelID, feedID,
	)
	if err != nil {
		return fmt.Errorf("remove feed from channel: %w", err)
	}
	return nil
}

func (s *ChannelStore) ListFeeds(ctx context.Context, channelID string) ([]*feed.Feed, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+feedCols+`
		FROM feeds
		JOIN channel_feeds cf ON cf.feed_id=feeds.id
		WHERE cf.channel_id=$1
		ORDER BY cf.added_at`,
		channelID,
	)
	if err != nil {
		return nil, fmt.Errorf("list channel feeds: %w", err)
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
