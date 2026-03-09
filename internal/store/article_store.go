package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laserfeed/laserfeed/internal/domain/article"
)

type ArticleStore struct {
	db *pgxpool.Pool
}

func NewArticleStore(db *pgxpool.Pool) *ArticleStore {
	return &ArticleStore{db: db}
}

const articleCols = `id, feed_id, guid, title, url, author, description, content,
	thumbnail_url, published_at, fetched_at, is_filtered_out, created_at`

func scanArticle(row interface{ Scan(...any) error }) (*article.Article, error) {
	a := &article.Article{}
	err := row.Scan(
		&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.URL, &a.Author,
		&a.Description, &a.Content, &a.ThumbnailURL,
		&a.PublishedAt, &a.FetchedAt, &a.IsFilteredOut, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ArticleStore) Upsert(ctx context.Context, a *article.Article) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO articles (feed_id, guid, title, url, author, description, content,
			thumbnail_url, published_at, fetched_at, is_filtered_out)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (feed_id, guid) DO UPDATE SET
			title=EXCLUDED.title, url=EXCLUDED.url, author=EXCLUDED.author,
			description=EXCLUDED.description, content=EXCLUDED.content,
			thumbnail_url=EXCLUDED.thumbnail_url, published_at=EXCLUDED.published_at,
			fetched_at=EXCLUDED.fetched_at, is_filtered_out=EXCLUDED.is_filtered_out`,
		a.FeedID, a.GUID, a.Title, a.URL, a.Author, a.Description, a.Content,
		a.ThumbnailURL, a.PublishedAt, a.FetchedAt, a.IsFilteredOut,
	)
	if err != nil {
		return fmt.Errorf("upsert article: %w", err)
	}
	return nil
}

func (s *ArticleStore) ListByFeedID(ctx context.Context, feedID string, includeFiltered bool, limit, offset int) ([]*article.Article, error) {
	filter := "AND is_filtered_out=false"
	if includeFiltered {
		filter = ""
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+articleCols+` FROM articles
		WHERE feed_id=$1 `+filter+`
		ORDER BY published_at DESC LIMIT $2 OFFSET $3`,
		feedID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list articles by feed: %w", err)
	}
	defer rows.Close()
	var articles []*article.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (s *ArticleStore) ListByFeedIDs(ctx context.Context, feedIDs []string, limit, offset int) ([]*article.Article, error) {
	if len(feedIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(feedIDs))
	args := make([]any, len(feedIDs)+2)
	for i, id := range feedIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	args[len(feedIDs)] = limit
	args[len(feedIDs)+1] = offset
	query := fmt.Sprintf(
		`SELECT `+articleCols+` FROM articles
		WHERE feed_id IN (%s) AND is_filtered_out=false
		ORDER BY published_at DESC LIMIT $%d OFFSET $%d`,
		strings.Join(placeholders, ","), len(feedIDs)+1, len(feedIDs)+2,
	)
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list articles by feed ids: %w", err)
	}
	defer rows.Close()
	var articles []*article.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (s *ArticleStore) ListRecent(ctx context.Context, limit, offset int) ([]*article.Article, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+articleCols+` FROM articles
		WHERE is_filtered_out=false
		ORDER BY published_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list recent articles: %w", err)
	}
	defer rows.Close()
	var articles []*article.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (s *ArticleStore) CountByFeedID(ctx context.Context, feedID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM articles WHERE feed_id=$1`, feedID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count articles: %w", err)
	}
	return count, nil
}

func (s *ArticleStore) DeleteOldest(ctx context.Context, feedID string, keepCount int) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM articles WHERE feed_id=$1 AND id NOT IN (
			SELECT id FROM articles WHERE feed_id=$1 ORDER BY published_at DESC LIMIT $2
		)`,
		feedID, keepCount,
	)
	if err != nil {
		return fmt.Errorf("delete oldest articles: %w", err)
	}
	return nil
}
