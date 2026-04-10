# Flyover — Specification

## Overview

**Flyover** is a personal content stream for a single technical curator. It pulls posts from curated blogs via RSS (with Readability-based scraping fallback), generates structured LLM summaries via OpenAI models, and presents them in a beautiful stream view for quick triage.

The site is publicly viewable by default so the curator can access and share it from anywhere, but it is not a multi-user product. Source management and post pushing require an API key.

Flyover is optimized for links + summaries, not full-text reading. For blog posts, the app stores the original link, metadata, tags, and generated summaries. For pushed posts, external automations may send raw content to summarize, but that content is used transiently and is not persisted after summarization.

---

## Architecture

**Single Go binary** with embedded frontend assets. One service to deploy.

```
┌─────────────────────────────────────────────┐
│              Flyover (Go binary)             │
│                                              │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │ HTTP API  │  │ Poller   │  │ Summarizer│  │
│  │ (Chi)     │  │ (Cron)   │  │ (OpenAI)  │  │
│  └──────────┘  └──────────┘  └───────────┘  │
│       │              │              │        │
│       └──────────────┴──────────────┘        │
│                      │                       │
│              ┌───────┴───────┐               │
│              │   Postgres    │               │
│              └───────────────┘               │
│                                              │
│  ┌──────────────────────────────────────┐    │
│  │  Embedded SPA (React + Vite + TW4)   │    │
│  └──────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

### Stack

| Layer      | Choice                            | Why                                                    |
|------------|-----------------------------------|--------------------------------------------------------|
| Backend    | **Go 1.22+**                      | Fast, single binary, great concurrency for polling     |
| Router     | **Chi**                           | Lightweight, idiomatic Go router                       |
| Database   | **PostgreSQL 16**                 | Reliable, works on Railway/Coolify/anywhere             |
| ORM        | **sqlc**                          | Type-safe SQL, no runtime overhead                     |
| Migrations | **golang-migrate**                | Simple, file-based migrations                          |
| Frontend   | **React 19 + Vite + Tailwind 4** | Embedded into Go binary at build time via `embed.FS`   |
| LLM        | **OpenAI (configurable model)** | Structured JSON output for summaries; default `gpt-5.4` |

---

## Data Model

### `sources`

| Column        | Type         | Notes                              |
|---------------|--------------|-------------------------------------|
| id            | UUID (PK)    | Auto-generated                      |
| kind          | TEXT         | `rss` or `push`                     |
| name          | TEXT         | Display name (e.g. "Lilian Weng")   |
| site_url      | TEXT NULL    | Blog home URL or source landing page |
| feed_url      | TEXT NULL    | RSS/Atom URL if applicable          |
| icon_url      | TEXT NULL    | Favicon or avatar                   |
| is_active     | BOOLEAN      | Default true                        |
| created_at    | TIMESTAMPTZ  |                                     |
| last_polled   | TIMESTAMPTZ NULL | Only used for `rss` sources     |
| metadata      | JSONB NULL   | Optional source-level metadata      |

### `posts`

| Column           | Type         | Notes                                         |
|------------------|--------------|------------------------------------------------|
| id               | UUID (PK)    |                                                |
| source_id        | UUID (FK)    | Required — every post belongs to a source      |
| title            | TEXT         |                                                |
| url              | TEXT UNIQUE  | Literal dedupe key for v1                       |
| author           | TEXT NULL    |                                                |
| summary_short    | TEXT NULL    | 2-sentence card summary                        |
| summary_long     | TEXT NULL    | 5-6 sentence detail summary                    |
| summary_status   | TEXT         | `pending`, `ready`, or `failed`                |
| summary_attempts | INTEGER      | Incremented on each summarization attempt      |
| summary_next_attempt_at | TIMESTAMPTZ NULL | When the next retry becomes eligible |
| summary_error    | TEXT NULL    | Last failure reason for observability          |
| summary_last_error_kind | TEXT NULL | `transient`, `permanent`, or `validation` |
| summarized_at    | TIMESTAMPTZ NULL | Set when summaries are generated          |
| tags             | TEXT[]       | 0-3 free-form topic tags                       |
| image_url        | TEXT NULL    | Hero/preview image                             |
| published_at     | TIMESTAMPTZ  | Original publish date                          |
| created_at       | TIMESTAMPTZ  | When we ingested it                            |
| metadata         | JSONB NULL   | Arbitrary extra data from push API             |
| search_vector    | TSVECTOR     | Auto-generated from title + summaries + tags. Updated via trigger. |

**Search index**: A GIN index on `search_vector` enables fast full-text keyword search across post titles, summaries, and tags.

**Important**: Flyover does **not** persist full article text in the `posts` table. Any fetched or pushed content used for summarization is handled transiently and discarded after processing.

### `api_keys`

| Column     | Type         | Notes                          |
|------------|--------------|--------------------------------|
| id         | UUID (PK)    |                                |
| key_hash   | TEXT         | SHA-256 of the key             |
| name       | TEXT         | e.g. "primary-admin"           |
| created_at | TIMESTAMPTZ  |                                |
| last_used_at | TIMESTAMPTZ NULL | Optional audit field      |

For v1, the system is expected to have a single long-lived admin key. The table exists mainly so the app can hash and verify the key cleanly, and leaves room for future rotation if needed.

---

## Features

### 1. Feed Ingestion (Poller)

- **Schedule**: Runs once daily at a configurable hour (default 3 AM UTC, set via `POLL_HOUR`).
- **Initial backfill**: On first-ever startup (when the posts table is empty), enqueue a one-time backfill job for all `rss` sources covering the last 60 days of posts. Does not run again on subsequent restarts.
- **Boot behavior**: The HTTP server starts immediately; backfill runs in the background and must not block the app from becoming available.
- **Scope**: Only `sources.kind = 'rss'` are polled. `push` sources create posts via API only.
- **RSS/Atom discovery**: For each RSS source, try `feed_url` first. If null, attempt auto-discovery from the site URL (`<link rel="alternate" type="application/rss+xml">`).
- **Scraping fallback**: If no RSS feed is found, use **go-readability** (Mozilla Readability port) to extract article content from the source page. No manual CSS selector config needed in v1.
- **Deduplication**: Skip posts where `url` already exists in the DB.
- **Backfill semantics**: The initial backfill job requests as many entries as the feed exposes within the last 60 days, inserts any missing posts, and skips anything already present.
- **Content fetching**: For RSS posts with only a snippet, fetch the full page and extract article content using go-readability.
- **Transient content pipeline**: Extracted article text is used only for summarization and is not stored in Postgres.
- **Failed extraction**: If content extraction fails entirely, still add the post with title + URL only. It remains visible in the stream with `summary_status = 'failed'`.
- **Concurrency**: Poll all RSS sources concurrently (bounded goroutine pool).

### 2. LLM Summarization

- **Role in product**: LLM summaries are a core feature. Flyover is designed to help the curator decide what to read without first reading the full article.
- **Trigger**: After a new post is ingested, attempt summarization if transient source content is available.
- **Content source**:
  - For RSS posts, fetch/extract the article body transiently at ingestion time.
  - For pushed posts, accept `content` in the API request and use it transiently for summarization.
  - In both cases, discard raw content after summarization completes or fails.
- **Model selection**: Summarization uses an OpenAI model configured at runtime. Default is `gpt-5.4`, but the model should be switchable via config without schema or code-path changes.
- **Output format**: Single API call per post using structured JSON output:
  ```json
  {
    "short": "2-sentence card summary",
    "long": "5-6 sentence detailed summary",
    "tags": ["machine-learning", "transformers"]
  }
  ```
- **Prompt style**: Focus on:
  1. The author's main point and intent/target audience
  2. The single biggest takeaway for the reader
  3. Up to 3 free-form topic tags (short, lowercase, hyphenated)
  - NOT generic "this post is about..." descriptions.
- **Rate limiting**: Process sequentially with a small delay to stay within rate limits.
- **Failure classification**:
  - **Transient**: rate limits, timeouts, temporary OpenAI/API errors, temporary network failures
  - **Validation**: malformed JSON or response shape mismatch
  - **Permanent**: empty/unusable extracted content, unsupported input, repeated validation failure after repair attempts
- **Retry logic**:
  - Retry `transient` failures up to 5 total attempts using exponential backoff with jitter.
  - Retry `validation` failures immediately up to 2 additional times with a stricter repair prompt before falling back to normal backoff.
  - Do not retry `permanent` failures beyond the current attempt; mark the post as `failed`.
  - Store `summary_attempts`, `summary_error`, `summary_last_error_kind`, and `summary_next_attempt_at` so retries are observable and resumable.
  - The daily cron (same schedule as the poller) also sweeps for any post with `summary_status = 'pending'` and `summary_next_attempt_at <= now()`. Pushed posts may wait up to 24h for summarization if not provided inline.
- **Fallback**: Posts without summaries still appear in the feed. The card shows the title, source, and any existing metadata, even if no summary is available.

### 3. Push API

```
POST /api/v1/posts
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "source_id": "0a1b2c3d-...",                                 // required, must reference an existing push source
  "title": "Paper: Attention Is Still All You Need",           // required
  "url": "https://arxiv.org/abs/2XXX.XXXXX",                   // required
  "content": "Full text or HTML of the post...",               // optional, used transiently for summarization only
  "summary_short": "Short card summary...",                    // optional (skips LLM if both provided)
  "summary_long": "Longer detail summary...",                  // optional (skips LLM if both provided)
  "author": "Codex ArXiv Bot",                                 // optional
  "tags": ["ml-research", "agents"],                           // optional, max 3, free-form
  "image_url": "https://...",                                  // optional
  "published_at": "2026-04-09T00:00:00Z",                     // optional (defaults to now)
  "metadata": { "p_value": 0.003, "verdict": "legit" }        // optional (arbitrary JSON)
}
```

- **Auth**: `Authorization: Bearer <key>` header. Key is hashed and compared against `api_keys` table.
- **Behavior**: If both `summary_short` and `summary_long` are provided, skip LLM summarization and mark the post `ready`. If only one or neither is provided, queue for summarization using the provided `content` when available.
- **Content persistence**: `content` is accepted for pushed posts but is never stored after summarization completes or fails.
- **Response**: `201 Created` with the post object, or `409 Conflict` if URL already exists.

### 4. Frontend — Stream View

**Single-page app** served from the Go binary. Light mode only.

#### Layout
- **Header**: "Flyover" branding, search bar, filter chips.
- **Stream**: Vertical feed of post cards, centered column, sorted by `published_at` descending (newest first). When a search query is active, sorted by relevance instead.
- **Infinite scroll**: Load 20 posts at a time, fetch more on scroll.
- **Loading state**: Skeleton cards matching the real card shape with pulse animation.

#### Post Card Design

```
┌─────────────────────────────────────────────────┐
│  ● [Source Icon]  Source Name  ·  Apr 9, 2026    │
│                                                   │
│  Post Title That Can Wrap to Two Lines            │
│                                                   │
│  Short LLM summary — the author's main point      │
│  and the single biggest takeaway.                  │
│                                                   │
│  [tag] [tag] [tag]                                │
└─────────────────────────────────────────────────┘
```

- Clean card with subtle border/shadow, generous padding, rounded corners.
- **Recent indicator**: Small green-blue dot next to the source icon for posts published within the last 7 days.
- Source icon + name on top. All posts display their associated source name, regardless of whether they came from RSS polling or a push automation.
- Title in bold, 1-2 lines max.
- `summary_short` in muted text.
- Tags as small pills (max 3 per post).
- Clicking a summarized card opens the **detail modal**.
- Clicking a post with `summary_status = 'failed'` opens the original URL directly in a new tab.
- Smooth fade-in animation as cards enter viewport (Intersection Observer).
- Optional: hero image at top of card if `image_url` exists.

#### Search

- Search bar in the header. Debounced input (300ms) triggers a full-text keyword search.
- Uses Postgres `tsvector` search across post titles, short summaries, long summaries, and tags.
- Results replace the stream, sorted by relevance (`ts_rank`).
- Filter chips still work in combination with search (e.g. search "transformers" + source filter "Lilian Weng").
- Clear search returns to the default chronological feed.

#### Filter Chips (Top Bar)

Horizontal row of filter chips above the feed:
- **Source chips**: One per source + "All" (default active). Click to filter by source.
- **Tag chips**: Populated from tags present in the current result set. Click to filter by tag.
- Chips are scrollable horizontally if they overflow.
- Active chip has filled background, inactive chips are outlined.

Because Flyover is expected to have around 20 sources max, source chips remain practical in v1.

#### Detail Modal

Triggered by clicking any post card. Appears as a centered modal with dimmed/blurred backdrop.

**Contents:**
- Post title (large)
- Source name + author + publish date
- `summary_long` (the 5-6 sentence extended summary)
- Tags as pills
- Prominent **"Read Original →"** button linking to the original post URL (opens in new tab)
- Close button (X) in top right + click backdrop to close + Escape key

The modal does NOT display the full article — it's a richer preview with the extended summary and metadata. If a post has no summary because summarization failed, the app skips the modal and links straight to the original article.

#### Theming & Visual Style

- **Light mode only**.
- **Color palette**: Green-blue to indigo gradient accents. Think teal (#0D9488) through indigo (#6366F1).
- Apple-esque aesthetic: rounded corners (lg/xl), subtle shadows, clean sans-serif typography (Inter or system font stack).
- Generous whitespace, max content width ~720px.
- Smooth transitions on hover/focus states.
- Cards have a subtle hover lift effect.

### 5. Source Management

- **Initial sources**: Seeded from config on first boot.
- **Source kinds**:
  - `rss` sources are polled on schedule.
  - `push` sources exist so automations can publish into the stream under a stable source identity.
- **Add/remove via API**: `POST /api/v1/sources` and `DELETE /api/v1/sources/:id` require API key auth.
- **No UI for source management** — it's an admin action done via API.
- When adding an RSS source via API, the server auto-discovers the RSS feed URL.
- When adding a push source via API, no feed discovery is needed.

### 6. API Endpoints

| Method | Path                  | Auth Required | Description                    |
|--------|-----------------------|---------------|--------------------------------|
| GET    | /api/v1/posts         | No            | List posts (paginated, filterable) |
| GET    | /api/v1/posts/:id     | No            | Get single post                |
| POST   | /api/v1/posts         | Yes           | Push a post                    |
| GET    | /api/v1/sources       | No            | List all sources               |
| POST   | /api/v1/sources       | Yes           | Add a new source               |
| DELETE | /api/v1/sources/:id   | Yes           | Remove a source                |
| GET    | /                     | No            | Serve the SPA                  |

Query params for `GET /api/v1/posts`:
- `page` (default 1)
- `per_page` (default 20, max 100)
- `source_id` — filter by source
- `tag` — filter by tag
- `q` — full-text search query (uses `tsvector`; when present, results sorted by relevance)
- `before` / `after` — date range

`GET /api/v1/posts` returns only persisted metadata, summaries, and links. It does not return full fetched or pushed article content because Flyover does not store that content.

---

## Configuration (Environment Variables)

| Variable          | Required | Default     | Description                          |
|-------------------|----------|-------------|--------------------------------------|
| DATABASE_URL      | Yes      | —           | Postgres connection string           |
| OPENAI_API_KEY    | Yes      | —           | For OpenAI summaries                |
| LLM_MODEL         | No       | `gpt-5.4`   | OpenAI model used for summarization |
| API_KEY           | Yes      | —           | Single push/admin API key (hashed on first boot) |
| BACKFILL_DAYS     | No       | 60          | Number of days of RSS history to backfill on first startup |
| POLL_HOUR         | No       | 3           | Hour (0-23 UTC) to run daily poll    |
| PORT              | No       | 8080        | HTTP server port                     |

---

## Deployment

**Docker-first**. Single `Dockerfile` producing a minimal image:

```dockerfile
# Build frontend
FROM node:20 AS frontend
WORKDIR /app/frontend
COPY frontend/ .
RUN npm ci && npm run build

# Build Go binary
FROM golang:1.22 AS backend
WORKDIR /app
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -o /flyover ./cmd/server

# Runtime
FROM alpine:3.19
COPY --from=backend /flyover /flyover
EXPOSE 8080
CMD ["/flyover"]
```

Works on:
- **Railway**: Add Postgres plugin, set env vars, deploy from GitHub.
- **Coolify**: Docker compose with Postgres sidecar.
- **Any VPS**: `docker compose up`.

A `docker-compose.yml` is provided for local dev with Postgres.

---

## Initial Sources

| Name           | URL                              | RSS Feed                                      |
|----------------|----------------------------------|-----------------------------------------------|
| Lilian Weng    | https://lilianweng.github.io/    | https://lilianweng.github.io/index.xml        |
| swyx           | https://www.swyx.io/             | https://www.swyx.io/rss.xml                   |
| Eugene Yan     | https://eugeneyan.com/           | https://eugeneyan.com/rss/                     |
| Tania Rascia   | https://www.taniarascia.com/     | https://www.taniarascia.com/rss.xml            |

---

## Out of Scope (for now)

- Semantic / embedding-based search
- Multi-user / auth (beyond a single admin key)
- Email digests
- Read/unread tracking
- Bookmarking / saving posts
- Dark mode
- Mobile app
- UI for source management

These can be added later without a rewrite, but some would require schema and API expansion.
