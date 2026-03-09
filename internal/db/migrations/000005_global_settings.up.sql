CREATE TABLE global_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO global_settings VALUES
    ('user_agent', 'Mozilla/5.0 (X11; Linux x86_64; rv:124.0) Gecko/20100101 Firefox/124.0', NOW()),
    ('poll_interval_seconds', '3600', NOW()),
    ('image_mode', 'extract', NOW()),
    ('placeholder_image_url', '', NOW()),
    ('max_articles_per_feed', '500', NOW());
