DROP INDEX IF EXISTS idx_articles_feed_scrape_status;

ALTER TABLE feeds
    DROP COLUMN IF EXISTS scrape_max_age_days;

ALTER TABLE articles
    DROP COLUMN IF EXISTS scrape_status,
    DROP COLUMN IF EXISTS scrape_error;

DROP TYPE IF EXISTS scrape_status;
