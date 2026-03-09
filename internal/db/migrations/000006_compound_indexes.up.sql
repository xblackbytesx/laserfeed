-- Compound index for the common query pattern:
--   WHERE feed_id=$1 [AND is_filtered_out=false] ORDER BY published_at DESC
-- This covers ListByFeedID and is more efficient than the separate single-column indexes.
CREATE INDEX idx_articles_feed_filtered_pub
    ON articles(feed_id, is_filtered_out, published_at DESC);

-- Compound index for the channel feed output query:
--   WHERE feed_id IN (...) AND is_filtered_out=false ORDER BY published_at DESC
CREATE INDEX idx_articles_filtered_pub
    ON articles(is_filtered_out, published_at DESC);
