package api

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	Queries  *db.Queries
	Router   chi.Router
	FrontendFS fs.FS // nil in tests
}

func NewServer(queries *db.Queries, frontendFS fs.FS) *Server {
	s := &Server{
		Queries: queries,
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// Public API
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/posts", s.ListPosts)
		r.Get("/posts/{id}", s.GetPost)
		r.Get("/sources", s.ListSources)

		// Protected API
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(queries))
			r.Post("/posts", s.CreatePost)
			r.Post("/sources", s.CreateSource)
			r.Delete("/sources/{id}", s.DeleteSource)
		})
	})

	// Serve frontend SPA
	if frontendFS != nil {
		fileServer := http.FileServer(http.FS(frontendFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Try serving the file directly
			if _, err := fs.Stat(frontendFS, r.URL.Path[1:]); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
			// Fallback to index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	s.Router = r
	return s
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
