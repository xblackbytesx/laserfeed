-- Tombstones for articles removed by retention policies. Without them, an
-- article deleted by count/age retention but still listed in the source feed
-- would be re-ingested (and re-scraped) on every poll, then deleted again —
-- permanent fetch churn. The poller skips items whose (feed_id, guid) has a
-- tombstone. Rows are cascade-deleted with their feed.
CREATE TABLE article_tombstones (
    feed_id UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (feed_id, guid)
);
