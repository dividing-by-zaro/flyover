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
