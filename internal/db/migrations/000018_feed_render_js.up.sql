-- Per-feed toggle: render article pages in a headless browser (remote CDP
-- endpoint, see JS_RENDER_WS_URL) before content extraction, for sites that
-- build their content client-side.
ALTER TABLE feeds ADD COLUMN scrape_render_js BOOLEAN NOT NULL DEFAULT false;
