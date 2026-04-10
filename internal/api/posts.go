package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type CreatePostRequest struct {
	SourceID     string          `json:"source_id"`
	Title        string          `json:"title"`
	URL          string          `json:"url"`
	Content      *string         `json:"content,omitempty"`
	SummaryShort *string         `json:"summary_short,omitempty"`
	SummaryLong  *string         `json:"summary_long,omitempty"`
	Author       *string         `json:"author,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	ImageURL     *string         `json:"image_url,omitempty"`
	PublishedAt  *time.Time      `json:"published_at,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

func (s *Server) ListPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := int32((page - 1) * perPage)

	searchQuery := q.Get("q")

	if searchQuery != "" {
		params := db.SearchPostsParams{
			WebsearchToTsquery: searchQuery,
			Limit:              int32(perPage),
			Offset:             offset,
		}
		if sid := q.Get("source_id"); sid != "" {
			if parsed, err := uuid.Parse(sid); err == nil {
				params.SourceID = pgtype.UUID{Bytes: parsed, Valid: true}
			}
		}
		if tag := q.Get("tag"); tag != "" {
			params.Tag = pgtype.Text{String: tag, Valid: true}
		}

		posts, err := s.Queries.SearchPosts(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to search posts")
			return
		}
		writeJSON(w, http.StatusOK, posts)
		return
	}

	params := db.ListPostsParams{
		Limit:  int32(perPage),
		Offset: offset,
	}
	if sid := q.Get("source_id"); sid != "" {
		if parsed, err := uuid.Parse(sid); err == nil {
			params.SourceID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
	}
	if tag := q.Get("tag"); tag != "" {
		params.Tag = pgtype.Text{String: tag, Valid: true}
	}
	if b := q.Get("before"); b != "" {
		if t, err := time.Parse(time.RFC3339, b); err == nil {
			params.Before = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}
	if a := q.Get("after"); a != "" {
		if t, err := time.Parse(time.RFC3339, a); err == nil {
			params.After = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	posts, err := s.Queries.ListPosts(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list posts")
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *Server) GetPost(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	post, err := s.Queries.GetPost(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, post)
}

func (s *Server) CreatePost(w http.ResponseWriter, r *http.Request) {
	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" || req.URL == "" || req.SourceID == "" {
		writeError(w, http.StatusBadRequest, "source_id, title, and url are required")
		return
	}

	sourceID, err := uuid.Parse(req.SourceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid source_id")
		return
	}

	_, err = s.Queries.GetSource(r.Context(), sourceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "source not found")
		return
	}

	exists, err := s.Queries.PostExistsByURL(r.Context(), req.URL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check URL uniqueness")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "post with this URL already exists")
		return
	}

	summaryStatus := "pending"
	if req.SummaryShort != nil && req.SummaryLong != nil {
		summaryStatus = "ready"
	}

	publishedAt := time.Now()
	if req.PublishedAt != nil {
		publishedAt = *req.PublishedAt
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	if len(tags) > 3 {
		tags = tags[:3]
	}

	params := db.CreatePostParams{
		SourceID:      sourceID,
		Title:         req.Title,
		Url:           req.URL,
		SummaryStatus: summaryStatus,
		Tags:          tags,
		PublishedAt:   publishedAt,
	}
	if req.Author != nil {
		params.Author = pgtype.Text{String: *req.Author, Valid: true}
	}
	if req.SummaryShort != nil {
		params.SummaryShort = pgtype.Text{String: *req.SummaryShort, Valid: true}
	}
	if req.SummaryLong != nil {
		params.SummaryLong = pgtype.Text{String: *req.SummaryLong, Valid: true}
	}
	if req.ImageURL != nil {
		params.ImageUrl = pgtype.Text{String: *req.ImageURL, Valid: true}
	}
	if req.Metadata != nil {
		params.Metadata = req.Metadata
	}

	post, err := s.Queries.CreatePost(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create post")
		return
	}

	writeJSON(w, http.StatusCreated, post)
}
