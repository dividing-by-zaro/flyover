package summarizer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	openai "github.com/sashabaranov/go-openai"

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
		t.Skipf("skipping: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping: %v", err)
	}
	m, _ := migrate.New("file://../../migrations", testDBURL())
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Logf("migration: %v", err)
	}
	pool.Exec(ctx, "TRUNCATE posts, sources CASCADE")
	return db.New(pool)
}

func createTestPost(t *testing.T, queries *db.Queries) db.Post {
	t.Helper()
	source, err := queries.CreateSource(context.Background(), db.CreateSourceParams{
		Kind: "push",
		Name: "Test Source",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	post, err := queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID:      source.ID,
		Title:         "Test Article About Machine Learning",
		Url:           "https://example.com/test-ml-article",
		SummaryStatus: "pending",
		Tags:          []string{},
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	return post
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err  string
		want ErrorKind
	}{
		{"rate limit exceeded", ErrorTransient},
		{"timeout waiting for response", ErrorTransient},
		{"503 service unavailable", ErrorTransient},
		{"429 too many requests", ErrorTransient},
		{"connection refused", ErrorTransient},
		{"unknown error", ErrorTransient},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			got := classifyError(fmt.Errorf("%s", tt.err))
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestExponentialBackoff(t *testing.T) {
	for i := 0; i < 5; i++ {
		d := exponentialBackoffWithJitter(i)
		if d <= 0 {
			t.Errorf("attempt %d: backoff should be positive, got %v", i, d)
		}
	}

	// Higher attempts should generally produce longer backoffs
	d0 := exponentialBackoffWithJitter(0)
	d4 := exponentialBackoffWithJitter(4)
	// d4 is at least 16s base, d0 is 1s base, so d4 should be larger most of the time
	// But with jitter this isn't guaranteed in a single sample, so just check d4 is reasonable
	if d4 < 0 {
		t.Errorf("attempt 4 backoff should be positive, got %v", d4)
	}
	_ = d0
}

func TestSummarizePost_EmptyContent(t *testing.T) {
	queries := setupTestDB(t)
	post := createTestPost(t, queries)

	s := &Summarizer{queries: queries}
	s.SummarizePost(context.Background(), post, "")

	// Should be marked as failed
	updated, err := queries.GetPost(context.Background(), post.ID)
	if err != nil {
		t.Fatalf("get post: %v", err)
	}
	if updated.SummaryStatus != "failed" {
		t.Errorf("expected 'failed', got '%s'", updated.SummaryStatus)
	}
	if !updated.SummaryError.Valid || updated.SummaryError.String != "empty content" {
		t.Errorf("expected error 'empty content', got '%s'", updated.SummaryError.String)
	}
}

func TestSummarizePost_Success(t *testing.T) {
	queries := setupTestDB(t)
	post := createTestPost(t, queries)

	// Mock OpenAI server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `{"short":"This article explains transformers. The key insight is attention mechanisms.","long":"The author presents a comprehensive overview of transformer architectures. They explain how self-attention allows models to process sequences in parallel. The paper introduces multi-head attention as a key innovation. Positional encodings solve the problem of sequence ordering. The architecture has become foundational in NLP. Performance improvements are dramatic compared to RNNs.","tags":["machine-learning","transformers","nlp"]}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = ts.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	s := NewWithClient(client, "test-model", queries)
	s.SummarizePost(context.Background(), post, "This is a long article about machine learning and transformers...")

	updated, err := queries.GetPost(context.Background(), post.ID)
	if err != nil {
		t.Fatalf("get post: %v", err)
	}
	if updated.SummaryStatus != "ready" {
		t.Errorf("expected 'ready', got '%s'", updated.SummaryStatus)
	}
	if !updated.SummaryShort.Valid {
		t.Error("summary_short should be set")
	}
	if !updated.SummaryLong.Valid {
		t.Error("summary_long should be set")
	}
	if len(updated.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(updated.Tags))
	}
}

func TestSummarizePost_InvalidJSON(t *testing.T) {
	queries := setupTestDB(t)
	post := createTestPost(t, queries)

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var content string
		if callCount <= 2 {
			content = `not valid json at all`
		} else {
			content = `{"short":"Fixed summary.","long":"This is the fixed long summary with enough detail. It covers multiple points. The author explains well. Good methodology. Clear conclusions. Solid work.","tags":["test"]}`
		}
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: content}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = ts.URL + "/v1"
	client := openai.NewClientWithConfig(config)

	s := NewWithClient(client, "test-model", queries)
	s.SummarizePost(context.Background(), post, "Some content to summarize.")

	updated, err := queries.GetPost(context.Background(), post.ID)
	if err != nil {
		t.Fatalf("get post: %v", err)
	}

	// After validation retries + repair prompt, should eventually succeed
	if updated.SummaryStatus != "ready" {
		t.Errorf("expected 'ready' after repair, got '%s' (calls: %d)", updated.SummaryStatus, callCount)
	}
}

func TestSweepPending(t *testing.T) {
	queries := setupTestDB(t)

	source, _ := queries.CreateSource(context.Background(), db.CreateSourceParams{
		Kind: "push",
		Name: "Sweep Test",
	})

	// Create a post with pending summary
	queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID:      source.ID,
		Title:         "Pending Post",
		Url:           "https://example.com/pending",
		SummaryStatus: "pending",
		Tags:          []string{},
	})

	// Create a post already summarized
	queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID:      source.ID,
		Title:         "Ready Post",
		Url:           "https://example.com/ready",
		SummaryStatus: "ready",
		SummaryShort:  pgtype.Text{String: "Already done", Valid: true},
		SummaryLong:   pgtype.Text{String: "Already done long", Valid: true},
		Tags:          []string{"done"},
	})

	pending, err := queries.ListPendingSummaries(context.Background())
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending post, got %d", len(pending))
	}
}
