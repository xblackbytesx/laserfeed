-- Raw Cookie header value sent with full-content scrape requests.
-- Format: "name=value; name2=value2" (copy from browser DevTools → Network → Cookie header).
ALTER TABLE feeds ADD COLUMN scrape_cookies TEXT;
