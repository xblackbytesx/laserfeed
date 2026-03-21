package repository

import (
	"context"
	"fmt"

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
	thumbnail_url, published_at, fetched_at, is_filtered_out, created_at,
	scrape_status, COALESCE(scrape_error, '')`

func scanArticle(row interface{ Scan(...any) error }) (*article.Article, error) {
	a := &article.Article{}
	var scrapeStatusStr string
	err := row.Scan(
		&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.URL, &a.Author,
		&a.Description, &a.Content, &a.ThumbnailURL,
		&a.PublishedAt, &a.FetchedAt, &a.IsFilteredOut, &a.CreatedAt,
		&scrapeStatusStr, &a.ScrapeError,
	)
	if err != nil {
		return nil, err
	}
	a.ScrapeStatus = article.ScrapeStatus(scrapeStatusStr)
	return a, nil
}

func (s *ArticleStore) Upsert(ctx context.Context, a *article.Article) error {
	scrapeErrPtr := (*string)(nil)
	if a.ScrapeError != "" {
		scrapeErrPtr = &a.ScrapeError
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO articles (feed_id, guid, title, url, author, description, content,
			thumbnail_url, published_at, fetched_at, is_filtered_out, scrape_status, scrape_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (feed_id, guid) DO UPDATE SET
			title=EXCLUDED.title, url=EXCLUDED.url, author=EXCLUDED.author,
			description=EXCLUDED.description,
			thumbnail_url = CASE WHEN EXCLUDED.thumbnail_url != '' THEN EXCLUDED.thumbnail_url ELSE articles.thumbnail_url END,
			published_at=EXCLUDED.published_at,
			fetched_at=EXCLUDED.fetched_at, is_filtered_out=EXCLUDED.is_filtered_out,
			-- Preserve a previously successful scrape: don't overwrite good content
			-- with a failed re-attempt (e.g. transient network error on next poll).
			content       = CASE WHEN articles.scrape_status = 'success' THEN articles.content       ELSE EXCLUDED.content       END,
			scrape_status = CASE WHEN articles.scrape_status = 'success' THEN articles.scrape_status ELSE EXCLUDED.scrape_status END,
			scrape_error  = CASE WHEN articles.scrape_status = 'success' THEN articles.scrape_error  ELSE EXCLUDED.scrape_error  END`,
		a.FeedID, a.GUID, a.Title, a.URL, a.Author, a.Description, a.Content,
		a.ThumbnailURL, a.PublishedAt, a.FetchedAt, a.IsFilteredOut,
		string(a.ScrapeStatus), scrapeErrPtr,
	)
	if err != nil {
		return fmt.Errorf("upsert article: %w", err)
	}
	return nil
}

func (s *ArticleStore) GetScrapedGUIDs(ctx context.Context, feedID string) (map[string]bool, error) {
	rows, err := s.db.Query(ctx,
		`SELECT guid FROM articles WHERE feed_id=$1 AND scrape_status='success'`,
		feedID,
	)
	if err != nil {
		return nil, fmt.Errorf("get scraped guids: %w", err)
	}
	defer rows.Close()
	guids := map[string]bool{}
	for rows.Next() {
		var guid string
		if err := rows.Scan(&guid); err != nil {
			return nil, err
		}
		guids[guid] = true
	}
	return guids, rows.Err()
}

func (s *ArticleStore) UpdateScrapeResult(ctx context.Context, id, content, errMsg string) error {
	status := article.ScrapeStatusSuccess
	var errPtr *string
	if errMsg != "" {
		status = article.ScrapeStatusFailed
		content = ""
		errPtr = &errMsg
	}
	_, err := s.db.Exec(ctx,
		`UPDATE articles SET content=$1, scrape_status=$2, scrape_error=$3 WHERE id=$4`,
		content, string(status), errPtr, id,
	)
	if err != nil {
		return fmt.Errorf("update scrape result: %w", err)
	}
	return nil
}

func (s *ArticleStore) UpdateThumbnail(ctx context.Context, id, thumbnailURL string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE articles SET thumbnail_url=$1 WHERE id=$2 AND (thumbnail_url = '' OR thumbnail_url IS NULL)`,
		thumbnailURL, id,
	)
	if err != nil {
		return fmt.Errorf("update thumbnail: %w", err)
	}
	return nil
}

func (s *ArticleStore) GetScrapeStats(ctx context.Context, feedID string) (*article.ScrapeStats, error) {
	rows, err := s.db.Query(ctx,
		`SELECT scrape_status, COUNT(*) FROM articles WHERE feed_id=$1 GROUP BY scrape_status`,
		feedID,
	)
	if err != nil {
		return nil, fmt.Errorf("get scrape stats: %w", err)
	}
	defer rows.Close()

	stats := &article.ScrapeStats{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch article.ScrapeStatus(status) {
		case article.ScrapeStatusSuccess:
			stats.Success = count
		case article.ScrapeStatusFailed:
			stats.Failed = count
		case article.ScrapeStatusNone:
			stats.None = count
		}
	}
	return stats, rows.Err()
}

func (s *ArticleStore) ListForReScrape(ctx context.Context, feedID string) ([]*article.ArticleRef, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, url FROM articles WHERE feed_id=$1 AND url != '' ORDER BY published_at DESC`,
		feedID,
	)
	if err != nil {
		return nil, fmt.Errorf("list for rescrape: %w", err)
	}
	defer rows.Close()
	var refs []*article.ArticleRef
	for rows.Next() {
		ref := &article.ArticleRef{}
		if err := rows.Scan(&ref.ID, &ref.URL); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (s *ArticleStore) PurgeScrapeContent(ctx context.Context, feedID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE articles SET content='', scrape_status='none', scrape_error=NULL
		WHERE feed_id=$1 AND scrape_status != 'none'`,
		feedID,
	)
	if err != nil {
		return fmt.Errorf("purge scrape content: %w", err)
	}
	return nil
}

func (s *ArticleStore) PurgeOldScrapeContent(ctx context.Context, feedID string, maxAgeDays int) error {
	_, err := s.db.Exec(ctx,
		`UPDATE articles SET content='', scrape_status='none', scrape_error=NULL
		WHERE feed_id=$1 AND scrape_status='success'
		  AND fetched_at < NOW() - ($2 * INTERVAL '1 day')`,
		feedID, maxAgeDays,
	)
	if err != nil {
		return fmt.Errorf("purge old scrape content: %w", err)
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
		return []*article.Article{}, nil
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+articleCols+` FROM articles
		WHERE feed_id = ANY($1) AND is_filtered_out=false
		ORDER BY published_at DESC LIMIT $2 OFFSET $3`,
		feedIDs, limit, offset,
	)
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
