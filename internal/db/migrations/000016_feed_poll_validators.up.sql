-- HTTP cache validators captured from the last successful poll, so the poller
-- can issue conditional GETs (If-None-Match / If-Modified-Since) and skip
-- re-downloading + re-parsing feeds that haven't changed.
ALTER TABLE feeds
    ADD COLUMN poll_etag          TEXT NOT NULL DEFAULT '',
    ADD COLUMN poll_last_modified TEXT NOT NULL DEFAULT '';
