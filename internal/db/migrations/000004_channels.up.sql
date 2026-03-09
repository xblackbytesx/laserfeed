CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE channel_feeds (
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    feed_id    UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, feed_id)
);

CREATE INDEX idx_channel_feeds_channel_id ON channel_feeds(channel_id);
CREATE INDEX idx_channel_feeds_feed_id ON channel_feeds(feed_id);
