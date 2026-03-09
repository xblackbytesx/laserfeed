package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laserfeed/laserfeed/internal/domain/settings"
)

type SettingsStore struct {
	db *pgxpool.Pool
}

func NewSettingsStore(db *pgxpool.Pool) *SettingsStore {
	return &SettingsStore{db: db}
}

func (s *SettingsStore) Get(ctx context.Context) (*settings.Settings, error) {
	rows, err := s.db.Query(ctx, `SELECT key, value FROM global_settings`)
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	defer rows.Close()

	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	poll, _ := strconv.Atoi(m["poll_interval_seconds"])
	max, _ := strconv.Atoi(m["max_articles_per_feed"])
	return &settings.Settings{
		UserAgent:           m["user_agent"],
		PollIntervalSeconds: poll,
		ImageMode:           m["image_mode"],
		PlaceholderImageURL: m["placeholder_image_url"],
		MaxArticlesPerFeed:  max,
	}, nil
}

func (s *SettingsStore) Set(ctx context.Context, key, value string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO global_settings (key, value, updated_at) VALUES ($1,$2,NOW())
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}
