CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Sources table
CREATE TABLE sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL CHECK (kind IN ('rss', 'push')),
    name TEXT NOT NULL,
    site_url TEXT,
    feed_url TEXT,
    icon_url TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_polled TIMESTAMPTZ,
    metadata JSONB
);

-- Posts table
CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    author TEXT,
    summary_short TEXT,
    summary_long TEXT,
    summary_status TEXT NOT NULL DEFAULT 'pending' CHECK (summary_status IN ('pending', 'ready', 'failed')),
    summary_attempts INTEGER NOT NULL DEFAULT 0,
    summary_next_attempt_at TIMESTAMPTZ,
    summary_error TEXT,
    summary_last_error_kind TEXT CHECK (summary_last_error_kind IN ('transient', 'permanent', 'validation')),
    summarized_at TIMESTAMPTZ,
    tags TEXT[] NOT NULL DEFAULT '{}',
    image_url TEXT,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB,
    search_vector TSVECTOR
);

-- Search vector trigger
CREATE FUNCTION posts_search_vector_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.summary_short, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(NEW.summary_long, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(array_to_string(NEW.tags, ' '), '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER posts_search_vector_trigger
    BEFORE INSERT OR UPDATE ON posts
    FOR EACH ROW
    EXECUTE FUNCTION posts_search_vector_update();

-- Indexes
CREATE INDEX idx_posts_source_id ON posts(source_id);
CREATE INDEX idx_posts_published_at ON posts(published_at DESC);
CREATE INDEX idx_posts_summary_status ON posts(summary_status);
CREATE INDEX idx_posts_search_vector ON posts USING GIN(search_vector);
CREATE INDEX idx_posts_tags ON posts USING GIN(tags);

-- API keys table
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);
