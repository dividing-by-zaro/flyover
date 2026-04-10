package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func createTestSource(t *testing.T, srv *Server, apiKey string) db.Source {
	t.Helper()
	body := `{"kind":"push","name":"Test Source"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create test source: %d %s", w.Code, w.Body.String())
	}

	var source db.Source
	json.NewDecoder(w.Body).Decode(&source)
	return source
}

func TestCreatePost_RequiresAuth(t *testing.T) {
	srv := setupTestServer(t)

	body := `{"source_id":"00000000-0000-0000-0000-000000000001","title":"Test","url":"https://example.com/1"}`
	req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCreatePost_Success(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-create-post"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	body := `{"source_id":"` + source.ID.String() + `","title":"Test Post","url":"https://example.com/test-post","author":"Test Author","tags":["ml","ai"]}`
	req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var post db.Post
	if err := json.NewDecoder(w.Body).Decode(&post); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if post.Title != "Test Post" {
		t.Errorf("expected title 'Test Post', got '%s'", post.Title)
	}
	if post.SummaryStatus != "pending" {
		t.Errorf("expected summary_status 'pending', got '%s'", post.SummaryStatus)
	}
}

func TestCreatePost_WithSummaries_MarksReady(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-post-ready"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	body := `{
		"source_id":"` + source.ID.String() + `",
		"title":"Summarized Post",
		"url":"https://example.com/summarized",
		"summary_short":"Short summary here.",
		"summary_long":"This is a longer summary with more details about the post."
	}`
	req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var post db.Post
	json.NewDecoder(w.Body).Decode(&post)
	if post.SummaryStatus != "ready" {
		t.Errorf("expected summary_status 'ready', got '%s'", post.SummaryStatus)
	}
}

func TestCreatePost_DuplicateURL_Returns409(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-dupe-url"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	body := `{"source_id":"` + source.ID.String() + `","title":"Post 1","url":"https://example.com/duplicate"}`

	// First post
	req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create failed: %d", w.Code)
	}

	// Duplicate
	req = httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestListPosts_Pagination(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-list-posts"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	// Create 3 posts
	for i := 0; i < 3; i++ {
		_, err := srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
			SourceID:      source.ID,
			Title:         "Post " + string(rune('A'+i)),
			Url:           "https://example.com/post-" + string(rune('a'+i)),
			SummaryStatus: "pending",
			Tags:          []string{},
			PublishedAt:   time.Now(),
		})
		if err != nil {
			t.Fatalf("failed to create post %d: %v", i, err)
		}
	}

	// Page 1 with per_page=2
	req := httptest.NewRequest("GET", "/api/v1/posts?per_page=2&page=1", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var posts []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 2 {
		t.Errorf("expected 2 posts on page 1, got %d", len(posts))
	}

	// Page 2
	req = httptest.NewRequest("GET", "/api/v1/posts?per_page=2&page=2", nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 post on page 2, got %d", len(posts))
	}
}

func TestGetPost_NotFound(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/posts/00000000-0000-0000-0000-000000000099", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetPost_Success(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-get-post"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	post, err := srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID:      source.ID,
		Title:         "Fetch Me",
		Url:           "https://example.com/fetch-me",
		SummaryStatus: "pending",
		Tags:          []string{"test"},
		SummaryShort:  pgtype.Text{String: "A short summary", Valid: true},
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/posts/"+post.ID.String(), nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["title"] != "Fetch Me" {
		t.Errorf("expected title 'Fetch Me', got '%v'", result["title"])
	}
	if result["source_name"] != "Test Source" {
		t.Errorf("expected source_name 'Test Source', got '%v'", result["source_name"])
	}
}

func TestCreatePost_TagsLimitedTo3(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-tags-limit"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	body := `{"source_id":"` + source.ID.String() + `","title":"Many Tags","url":"https://example.com/many-tags","tags":["a","b","c","d","e"]}`
	req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var post db.Post
	json.NewDecoder(w.Body).Decode(&post)
	if len(post.Tags) != 3 {
		t.Errorf("expected 3 tags max, got %d", len(post.Tags))
	}
}

func TestListPosts_FilterBySource(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-filter-source"
	seedAPIKey(t, srv.Queries, apiKey)

	source1 := createTestSource(t, srv, apiKey)

	// Create second source
	body := `{"kind":"push","name":"Other Source"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	var source2 db.Source
	json.NewDecoder(w.Body).Decode(&source2)

	// Create posts for each source
	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source1.ID, Title: "Source1 Post", Url: "https://example.com/s1",
		SummaryStatus: "pending", Tags: []string{}, PublishedAt: time.Now(),
	})
	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source2.ID, Title: "Source2 Post", Url: "https://example.com/s2",
		SummaryStatus: "pending", Tags: []string{}, PublishedAt: time.Now(),
	})

	// Filter by source1
	req = httptest.NewRequest("GET", "/api/v1/posts?source_id="+source1.ID.String(), nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	var posts []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 post filtered by source, got %d", len(posts))
	}
}

func TestListPosts_FilterByTag(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-filter-tag"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source.ID, Title: "ML Post", Url: "https://example.com/ml",
		SummaryStatus: "pending", Tags: []string{"machine-learning"}, PublishedAt: time.Now(),
	})
	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source.ID, Title: "Web Post", Url: "https://example.com/web",
		SummaryStatus: "pending", Tags: []string{"web-dev"}, PublishedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/v1/posts?tag=machine-learning", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	var posts []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 post filtered by tag, got %d", len(posts))
	}
}

func TestListPosts_FilterByDateRange(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-filter-date"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	oldDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source.ID, Title: "Old Post", Url: "https://example.com/old",
		SummaryStatus: "pending", Tags: []string{}, PublishedAt: oldDate,
	})
	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source.ID, Title: "New Post", Url: "https://example.com/new",
		SummaryStatus: "pending", Tags: []string{}, PublishedAt: newDate,
	})

	// Only posts after March
	req := httptest.NewRequest("GET", "/api/v1/posts?after=2026-03-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	var posts []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 post after March, got %d", len(posts))
	}

	// Only posts before February
	req = httptest.NewRequest("GET", "/api/v1/posts?before=2026-02-01T00:00:00Z", nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 post before Feb, got %d", len(posts))
	}
}

func TestSearchPosts_FullTextSearch(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-search"
	seedAPIKey(t, srv.Queries, apiKey)
	source := createTestSource(t, srv, apiKey)

	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source.ID, Title: "Understanding Transformer Architecture",
		Url: "https://example.com/transformers", SummaryStatus: "ready",
		SummaryShort: pgtype.Text{String: "Deep dive into attention mechanisms", Valid: true},
		SummaryLong:  pgtype.Text{String: "A comprehensive guide to transformers", Valid: true},
		Tags: []string{"machine-learning", "transformers"}, PublishedAt: time.Now(),
	})
	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source.ID, Title: "Building REST APIs with Go",
		Url: "https://example.com/go-apis", SummaryStatus: "ready",
		SummaryShort: pgtype.Text{String: "How to build HTTP APIs in Go", Valid: true},
		SummaryLong:  pgtype.Text{String: "A guide to building REST APIs", Valid: true},
		Tags: []string{"golang", "api"}, PublishedAt: time.Now(),
	})

	// Search for "transformer"
	req := httptest.NewRequest("GET", "/api/v1/posts?q=transformer", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var posts []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 result for 'transformer', got %d", len(posts))
	}
	if len(posts) > 0 {
		if posts[0]["title"] != "Understanding Transformer Architecture" {
			t.Errorf("wrong search result: %v", posts[0]["title"])
		}
	}

	// Search for "API" should find the Go post
	req = httptest.NewRequest("GET", "/api/v1/posts?q=API", nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 result for 'API', got %d", len(posts))
	}

	// Search with no results
	req = httptest.NewRequest("GET", "/api/v1/posts?q=kubernetes", nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 0 {
		t.Errorf("expected 0 results for 'kubernetes', got %d", len(posts))
	}
}

func TestSearchPosts_WithSourceFilter(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-search-filter"
	seedAPIKey(t, srv.Queries, apiKey)

	source1 := createTestSource(t, srv, apiKey)

	body := `{"kind":"push","name":"Blog B"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	var source2 db.Source
	json.NewDecoder(w.Body).Decode(&source2)

	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source1.ID, Title: "Machine Learning Guide",
		Url: "https://example.com/ml1", SummaryStatus: "ready",
		SummaryShort: pgtype.Text{String: "ML guide", Valid: true},
		Tags: []string{"ml"}, PublishedAt: time.Now(),
	})
	srv.Queries.CreatePost(context.Background(), db.CreatePostParams{
		SourceID: source2.ID, Title: "Machine Learning Tips",
		Url: "https://example.com/ml2", SummaryStatus: "ready",
		SummaryShort: pgtype.Text{String: "ML tips", Valid: true},
		Tags: []string{"ml"}, PublishedAt: time.Now(),
	})

	// Search "machine learning" filtered to source1
	req = httptest.NewRequest("GET", "/api/v1/posts?q=machine+learning&source_id="+source1.ID.String(), nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	var posts []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&posts)
	if len(posts) != 1 {
		t.Errorf("expected 1 result with source filter, got %d", len(posts))
	}
}
