package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

	poll, err := strconv.Atoi(m["poll_interval_seconds"])
	if err != nil || poll < 60 {
		poll = 3600
	}
	max, err := strconv.Atoi(m["max_articles_per_feed"])
	if err != nil || max < 1 {
		max = 500
	}

	return &settings.Settings{
		UserAgent:           m["user_agent"],
		PollIntervalSeconds: poll,
		ImageMode:           m["image_mode"],
		PlaceholderImageURL: m["placeholder_image_url"],
		MaxArticlesPerFeed:  max,
	}, nil
}

func (s *SettingsStore) SetAll(ctx context.Context, pairs map[string]string) error {
	if len(pairs) == 0 {
		return nil
	}
	args := make([]any, 0, len(pairs)*2)
	placeholders := make([]string, 0, len(pairs))
	i := 1
	for k, v := range pairs {
		placeholders = append(placeholders, fmt.Sprintf("($%d,$%d,NOW())", i, i+1))
		args = append(args, k, v)
		i += 2
	}
	query := `INSERT INTO global_settings (key, value, updated_at) VALUES ` +
		strings.Join(placeholders, ",") +
		` ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`
	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("set settings: %w", err)
	}
	return nil
}
