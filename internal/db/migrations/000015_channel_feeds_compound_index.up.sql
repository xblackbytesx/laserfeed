-- The channel feed-output query filters by channel_id and orders by added_at.
-- A compound index serves both in one scan; it also covers the lookups the
-- single-column channel_id index handled, so that one is dropped.
DROP INDEX IF EXISTS idx_channel_feeds_channel_id;
CREATE INDEX idx_channel_feeds_channel_added ON channel_feeds(channel_id, added_at);
