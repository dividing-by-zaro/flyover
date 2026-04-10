package api

import (
	"encoding/json"
	"net/http"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type CreateSourceRequest struct {
	Kind    string           `json:"kind"`
	Name    string           `json:"name"`
	SiteURL *string          `json:"site_url,omitempty"`
	FeedURL *string          `json:"feed_url,omitempty"`
	IconURL *string          `json:"icon_url,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

func (s *Server) ListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.Queries.ListSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sources")
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) CreateSource(w http.ResponseWriter, r *http.Request) {
	var req CreateSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Kind != "rss" && req.Kind != "push" {
		writeError(w, http.StatusBadRequest, "kind must be 'rss' or 'push'")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	params := db.CreateSourceParams{
		Kind: req.Kind,
		Name: req.Name,
	}
	if req.SiteURL != nil {
		params.SiteUrl = pgtype.Text{String: *req.SiteURL, Valid: true}
	}
	if req.FeedURL != nil {
		params.FeedUrl = pgtype.Text{String: *req.FeedURL, Valid: true}
	}
	if req.IconURL != nil {
		params.IconUrl = pgtype.Text{String: *req.IconURL, Valid: true}
	}
	if req.Metadata != nil {
		params.Metadata = req.Metadata
	}

	source, err := s.Queries.CreateSource(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create source")
		return
	}

	writeJSON(w, http.StatusCreated, source)
}

func (s *Server) DeleteSource(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	err = s.Queries.DeleteSource(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete source")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
