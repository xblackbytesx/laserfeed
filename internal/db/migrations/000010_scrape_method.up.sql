CREATE TYPE scrape_method AS ENUM ('readability', 'selector');

ALTER TABLE feeds ADD COLUMN scrape_method scrape_method NOT NULL DEFAULT 'readability';

-- Feeds that already have a content selector set were using selector-based extraction,
-- so migrate them to the explicit 'selector' method.
UPDATE feeds SET scrape_method = 'selector' WHERE scrape_selector IS NOT NULL AND scrape_selector != '';
