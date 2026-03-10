![description](./assets/img/laserfeed-banner.png)

# LaserFeed

Self-hostable RSS/Atom feed aggregator. Add feed sources to a shared pool, configure per-feed content scraping, thumbnail policies, and keyword filters, then compose **Channels** that each produce their own Atom output. Point any RSS reader at a channel URL and get a clean, curated feed.

**Stack:** Go 1.24 · Echo · templ SSR · HTMX · Shoelace · PostgreSQL 17 · Docker

---

## Features

- **Feed pool** — add any RSS/Atom source; configure poll interval, user-agent override
- **Full-content scraping** — fetch the actual article page and extract content via CSS or XPath selector (or best-effort auto-extraction); reader-view sanitisation strips ads and nav
- **Cookie support** — paste a raw `Cookie` header to bypass cookie walls on paywalled sites
- **Filter rules** — whitelist/blacklist by title, URL, description, or content with substring or glob patterns (`*`, `?`)
- **Channels** — named aggregates that combine any subset of feeds into a single Atom feed at `/channels/:slug/feed.rss`
- **Thumbnails** — automatic extraction from feed media tags; configurable fallback (extract from content, placeholder URL, or per-article identicon)
- **Content retention** — configurable max age for scraped content; manual purge and re-scrape controls

---

## Development

**Requirements:** Docker and Docker Compose.

```bash
git clone https://github.com/xblackbytesx/laserfeed.git
cd laserfeed
make dev
```

Open [http://localhost:8080](http://localhost:8080).

The dev stack uses [Air](https://github.com/air-verse/air) for hot-reload. Editing any `.go` or `.templ` file triggers an automatic rebuild — no manual restart needed. The app binary is rebuilt inside the container, so no local Go installation is required.

### Useful commands

| Command | Description |
|---|---|
| `make dev` | Start dev stack with hot-reload |
| `make logs` | Tail app logs |
| `make reset` | Full teardown + clean restart (runs new migrations) |
| `make down` | Stop all containers |

### When to `make reset`

Run `make reset` after pulling changes that add database migrations (new columns, tables, or types). It drops the dev volume, recreates it, and runs all migrations from scratch.

### Dev credentials

The dev stack uses hardcoded throwaway values — no `.env` file needed:

| Setting | Value |
|---|---|
| Database URL | `postgres://laserfeed:laserfeed@laserfeed-db:5432/laserfeed` |
| CSRF key | `dev-csrf-key-32-chars-minimum!!!` |
| Secure cookies | `false` (HTTP only in dev) |

---

## Production

### 1. Create a `.env` file

```bash
cp .env.example .env
```

Edit `.env`:

```env
# Required — generate with: openssl rand -base64 32
CSRF_AUTH_KEY=change-me-to-a-random-32-char-minimum-secret

# Required — strong password for the database
DB_PASSWORD=change-me-to-a-strong-password

# Required — the public URL your instance is reachable at
# Used to build self-links in Atom feeds
APP_BASE_URL=https://feeds.example.com

# Optional — defaults to true; set false only if running behind an HTTP proxy
# that handles TLS termination and you know what you're doing
# SECURE_COOKIES=true
```

Generate a secure CSRF key:

```bash
openssl rand -base64 32
```

### 2. Start the stack

```bash
make up
```

This builds the production image (Go binary, templ generation, Shoelace assets) and starts the app and database in detached mode. Migrations run automatically on startup.

```bash
# Check it started correctly
docker compose logs app
```

### 3. Verify

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

### Useful production commands

| Command | Description |
|---|---|
| `make up` | Build and start in detached mode |
| `make down` | Stop containers (data is preserved in the volume) |
| `make build` | Rebuild the image without starting |
| `docker compose logs -f app` | Follow app logs |

### Reverse proxy (recommended)

Run behind nginx or Caddy to handle TLS. Example minimal Caddyfile:

```
feeds.example.com {
    reverse_proxy localhost:8080
}
```

With TLS termination handled by the proxy, set `SECURE_COOKIES=true` (the default) in your `.env`.

### Upgrading

```bash
git pull
make up   # rebuilds image and restarts; migrations run automatically
```

If a new release adds database migrations, they run on the next startup — no manual steps needed.

### Backups

Data lives in the `pgdata` Docker volume. To dump the database:

```bash
docker compose exec db pg_dump -U laserfeed laserfeed > backup.sql
```

To restore:

```bash
docker compose exec -T db psql -U laserfeed laserfeed < backup.sql
```

---

## Configuration reference

All configuration is via environment variables.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `CSRF_AUTH_KEY` | Yes | — | CSRF signing key, minimum 32 characters |
| `APP_BASE_URL` | No | `http://localhost:8080` | Public URL, used in Atom feed self-links |
| `PORT` | No | `8080` | Port the HTTP server binds to |
| `SECURE_COOKIES` | No | `true` | Set `false` when running over plain HTTP (dev only) |

---

## Feed output URLs

| URL | Description |
|---|---|
| `/channels/:slug/feed.rss` | Atom feed for a specific channel |
| `/feed.rss` | All articles from all feeds (unfiltered by channel) |

Both endpoints return `Content-Type: application/atom+xml` and can be added directly to any RSS reader.

---

## Scraping tips

**Finding the right CSS selector**

Open the target article in your browser, right-click the main article body → Inspect, then copy the selector. Common patterns:

```
article
.article-body
.post-content
main
[role=main]
```

**Cookie walls (e.g. regional news sites)**

1. Open the site in your browser and log in or accept cookies
2. Open DevTools → Network → reload a page → click any request to the site → Request Headers → copy the `Cookie` value
3. Paste it into the "Cookie header" field on the feed edit page
4. Save, then trigger a re-scrape

Cookies are stored as plain text in the database. Don't use a shared LaserFeed instance for sites where your session cookie grants access to paid content.

**XPath selectors**

Use XPath when the content isn't cleanly addressable with CSS, e.g.:

```xpath
//article[@class='story-body']
//div[contains(@class,'post-content')]
```
