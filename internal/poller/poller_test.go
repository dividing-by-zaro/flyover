package poller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func testDBURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://localhost:5432/flyover_test?sslmode=disable"
}

func setupTestDB(t *testing.T) *db.Queries {
	t.Helper()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, testDBURL())
	if err != nil {
		t.Skipf("skipping: cannot connect to postgres: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping: cannot ping postgres: %v", err)
	}

	m, mErr := migrate.New("file://../../migrations", testDBURL())
	if mErr != nil {
		t.Fatalf("failed to create migrator: %v", mErr)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Logf("migration warning: %v", err)
	}

	_, _ = pool.Exec(ctx, "DELETE FROM posts")
	_, _ = pool.Exec(ctx, "DELETE FROM sources")

	return db.New(pool)
}

func TestDiscoverFeedURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<link rel="alternate" type="application/rss+xml" href="/feed.xml">
		</head><body>Hello</body></html>`)
	}))
	defer ts.Close()

	feedURL, err := discoverFeedURL(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := ts.URL + "/feed.xml"
	if feedURL != expected {
		t.Errorf("expected %s, got %s", expected, feedURL)
	}
}

func TestDiscoverFeedURL_AtomFeed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<link rel="alternate" type="application/atom+xml" href="https://example.com/atom.xml">
		</head><body>Hello</body></html>`)
	}))
	defer ts.Close()

	feedURL, err := discoverFeedURL(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if feedURL != "https://example.com/atom.xml" {
		t.Errorf("expected https://example.com/atom.xml, got %s", feedURL)
	}
}

func TestDiscoverFeedURL_NoFeed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>No Feed</title></head><body>Hello</body></html>`)
	}))
	defer ts.Close()

	_, err := discoverFeedURL(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected error when no feed found")
	}
}

func TestPollSource_WithFeed(t *testing.T) {
	queries := setupTestDB(t)

	// Serve a test RSS feed
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
		<rss version="2.0">
			<channel>
				<title>Test Blog</title>
				<item>
					<title>Test Article</title>
					<link>https://example.com/test-article</link>
					<pubDate>Thu, 10 Apr 2026 12:00:00 GMT</pubDate>
					<description>This is a test article about machine learning.</description>
				</item>
				<item>
					<title>Another Article</title>
					<link>https://example.com/another-article</link>
					<pubDate>Wed, 09 Apr 2026 12:00:00 GMT</pubDate>
					<description>Another interesting article.</description>
				</item>
			</channel>
		</rss>`)
	}))
	defer ts.Close()

	source, err := queries.CreateSource(context.Background(), db.CreateSourceParams{
		Kind:    "rss",
		Name:    "Test Blog",
		SiteUrl: pgtype.Text{String: "https://testblog.com", Valid: true},
		FeedUrl: pgtype.Text{String: ts.URL, Valid: true},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	p := New(queries, 60)

	var summarizedPosts []string
	p.Summarize = func(ctx context.Context, post db.Post, content string) {
		summarizedPosts = append(summarizedPosts, post.Title)
	}

	err = p.pollSource(context.Background(), source, 0)
	if err != nil {
		t.Fatalf("poll source: %v", err)
	}

	// Check posts were created
	posts, err := queries.ListPosts(context.Background(), db.ListPostsParams{
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list posts: %v", err)
	}

	if len(posts) != 2 {
		t.Errorf("expected 2 posts, got %d", len(posts))
	}
}

func TestPollSource_Deduplication(t *testing.T) {
	queries := setupTestDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
		<rss version="2.0">
			<channel>
				<title>Test</title>
				<item>
					<title>Same Article</title>
					<link>https://example.com/same</link>
					<pubDate>Thu, 10 Apr 2026 12:00:00 GMT</pubDate>
				</item>
			</channel>
		</rss>`)
	}))
	defer ts.Close()

	source, err := queries.CreateSource(context.Background(), db.CreateSourceParams{
		Kind:    "rss",
		Name:    "Dedup Test",
		FeedUrl: pgtype.Text{String: ts.URL, Valid: true},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	p := New(queries, 60)

	// Poll twice
	_ = p.pollSource(context.Background(), source, 0)
	_ = p.pollSource(context.Background(), source, 0)

	posts, _ := queries.ListPosts(context.Background(), db.ListPostsParams{
		Limit:  100,
		Offset: 0,
	})

	if len(posts) != 1 {
		t.Errorf("expected 1 post (deduped), got %d", len(posts))
	}
}

func TestBackfill_SkipsIfPostsExist(t *testing.T) {
	queries := setupTestDB(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
		<rss version="2.0">
			<channel><title>T</title>
				<item><title>A</title><link>https://example.com/a</link></item>
			</channel>
		</rss>`)
	}))
	defer ts.Close()

	source, _ := queries.CreateSource(context.Background(), db.CreateSourceParams{
		Kind:    "rss",
		Name:    "Backfill Test",
		FeedUrl: pgtype.Text{String: ts.URL, Valid: true},
	})
	_ = source

	// Create an existing post
	queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID:      source.ID,
		Title:         "Existing",
		Url:           "https://example.com/existing",
		SummaryStatus: "pending",
		Tags:          []string{},
	})

	p := New(queries, 60)
	err := p.Backfill(context.Background())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Should only have the existing post, no new ones from backfill
	posts, _ := queries.ListPosts(context.Background(), db.ListPostsParams{
		Limit:  100,
		Offset: 0,
	})
	if len(posts) != 1 {
		t.Errorf("expected 1 post (backfill skipped), got %d", len(posts))
	}
}

func TestSeedSources(t *testing.T) {
	queries := setupTestDB(t)

	err := SeedSources(context.Background(), queries)
	if err != nil {
		t.Fatalf("seed sources: %v", err)
	}

	sources, err := queries.ListSources(context.Background())
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}

	if len(sources) != 4 {
		t.Errorf("expected 4 seeded sources, got %d", len(sources))
	}

	// Run again - should not duplicate
	err = SeedSources(context.Background(), queries)
	if err != nil {
		t.Fatalf("seed sources again: %v", err)
	}

	sources, _ = queries.ListSources(context.Background())
	if len(sources) != 4 {
		t.Errorf("expected still 4 sources after re-seed, got %d", len(sources))
	}
}

func TestExtractContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		desc    string
		want    string
	}{
		{"content preferred", "full content", "short desc", "full content"},
		{"description fallback", "", "short desc", "short desc"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &gofeed.Item{
				Content:     tt.content,
				Description: tt.desc,
			}
			got := extractContent(item)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
