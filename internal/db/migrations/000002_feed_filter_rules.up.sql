CREATE TYPE rule_type AS ENUM ('whitelist', 'blacklist');
CREATE TYPE match_field AS ENUM ('title', 'url', 'content', 'description');

CREATE TABLE feed_filter_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_id UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    rule_type rule_type NOT NULL,
    match_field match_field NOT NULL,
    match_pattern TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_feed_filter_rules_feed_id ON feed_filter_rules(feed_id);
