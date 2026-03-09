CREATE TYPE scrape_status AS ENUM ('none', 'success', 'failed');

ALTER TABLE articles
    ADD COLUMN scrape_status scrape_status NOT NULL DEFAULT 'none',
    ADD COLUMN scrape_error  TEXT;

-- Per-feed retention period for scraped content (0 = keep forever)
ALTER TABLE feeds
    ADD COLUMN scrape_max_age_days INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_articles_feed_scrape_status ON articles(feed_id, scrape_status);
