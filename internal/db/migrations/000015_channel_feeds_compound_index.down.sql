DROP INDEX IF EXISTS idx_channel_feeds_channel_added;
CREATE INDEX idx_channel_feeds_channel_id ON channel_feeds(channel_id);
