package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dividing-by-zaro/flyover/internal/db"
)

func TestListSources_Empty(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/sources", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sources []db.Source
	if err := json.NewDecoder(w.Body).Decode(&sources); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(sources) != 0 {
		t.Errorf("expected 0 sources, got %d", len(sources))
	}
}

func TestCreateSource_RequiresAuth(t *testing.T) {
	srv := setupTestServer(t)

	body := `{"kind":"rss","name":"Test Blog","site_url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCreateSource_Success(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-for-sources"
	seedAPIKey(t, srv.Queries, apiKey)

	body := `{"kind":"rss","name":"Test Blog","site_url":"https://example.com","feed_url":"https://example.com/rss.xml"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var source db.Source
	if err := json.NewDecoder(w.Body).Decode(&source); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if source.Name != "Test Blog" {
		t.Errorf("expected name 'Test Blog', got '%s'", source.Name)
	}
	if source.Kind != "rss" {
		t.Errorf("expected kind 'rss', got '%s'", source.Kind)
	}
}

func TestCreateSource_InvalidKind(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-invalid-kind"
	seedAPIKey(t, srv.Queries, apiKey)

	body := `{"kind":"webhook","name":"Test"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateSource_PushKind(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-push-source"
	seedAPIKey(t, srv.Queries, apiKey)

	body := `{"kind":"push","name":"ArXiv Bot"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteSource_RequiresAuth(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/v1/sources/00000000-0000-0000-0000-000000000001", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestListSources_AfterCreate(t *testing.T) {
	srv := setupTestServer(t)
	apiKey := "test-key-list-sources"
	seedAPIKey(t, srv.Queries, apiKey)

	// Create a source
	body := `{"kind":"rss","name":"My Blog","site_url":"https://myblog.com"}`
	req := httptest.NewRequest("POST", "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}

	// List sources
	req = httptest.NewRequest("GET", "/api/v1/sources", nil)
	w = httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sources []db.Source
	if err := json.NewDecoder(w.Body).Decode(&sources); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(sources))
	}
}
