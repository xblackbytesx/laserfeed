CREATE TYPE image_mode AS ENUM ('none', 'extract', 'placeholder', 'random');
CREATE TYPE selector_type AS ENUM ('css', 'xpath');

CREATE TABLE feeds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    poll_interval_seconds INTEGER NOT NULL DEFAULT 3600,
    user_agent TEXT,
    scrape_full_content BOOLEAN NOT NULL DEFAULT false,
    scrape_selector TEXT,
    scrape_selector_type selector_type NOT NULL DEFAULT 'css',
    image_mode image_mode NOT NULL DEFAULT 'extract',
    placeholder_image_url TEXT,
    last_polled_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
