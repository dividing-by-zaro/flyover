## Flyover Push API

Base URL: `https://flyover-production.up.railway.app`

### Authentication

All write requests require a Bearer token in the `Authorization` header.

### Step 1: Create a push source

A push source is a named identity for your automation. Create one per bot/workflow.

```
POST /api/v1/sources
Authorization: Bearer <API_KEY>
Content-Type: application/json

{
  "kind": "push",
  "name": "ArXiv Bot"
}
```

Response: `201 Created` with a JSON object. Save the `id` — you'll need it for posting.

### Step 2: Post content

```
POST /api/v1/posts
Authorization: Bearer <API_KEY>
Content-Type: application/json

{
  "source_id": "<source UUID from step 1>",
  "title": "Paper: Attention Is Still All You Need",
  "url": "https://arxiv.org/abs/2XXX.XXXXX",
  "content": "Full text or HTML of the post. Used for LLM summarization, then discarded. Optional.",
  "summary_short": "One-sentence summary for the card view. Optional — if omitted, the LLM generates it.",
  "summary_long": "5-6 sentence detailed summary. Optional — if omitted, the LLM generates it.",
  "author": "Optional author name",
  "tags": ["ml-research", "agents"],
  "image_url": "https://example.com/hero.png",
  "published_at": "2026-04-09T00:00:00Z",
  "metadata": { "arbitrary": "json" }
}
```

**Required fields:** `source_id`, `title`, `url`

**Everything else is optional.** Here's how summarization works:

- If you provide both `summary_short` and `summary_long`: post is marked `ready` immediately, no LLM call.
- If you provide `content` but no summaries: the LLM generates summaries from the content, then the content is discarded.
- If you provide nothing: the post appears with title and link only. Summarization is attempted on the next daily sweep.

**Response:** `201 Created` with the post object, or `409 Conflict` if the URL already exists.

### Tags

- Max 3 per post
- Short, lowercase, hyphenated (e.g. `ml-research`, `web-dev`)
- If you omit tags and the LLM runs, it generates them automatically

### Dates

- `published_at` defaults to now if omitted
- Use ISO 8601 / RFC 3339 format

### Errors

| Status | Meaning |
|--------|---------|
| 201 | Post created |
| 400 | Missing required fields or invalid source_id |
| 401 | Missing or invalid API key |
| 409 | A post with this URL already exists |
