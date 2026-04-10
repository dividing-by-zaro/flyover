package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/dividing-by-zaro/flyover/internal/db"
)

type contextKey string

const apiKeyContextKey contextKey = "api_key_id"

func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h[:])
}

func AuthMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			hash := HashAPIKey(token)

			apiKey, err := queries.GetAPIKeyByHash(r.Context(), hash)
			if err != nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			// Update last_used_at in the background
			go func() {
				_ = queries.UpdateAPIKeyLastUsed(context.Background(), apiKey.ID)
			}()

			ctx := context.WithValue(r.Context(), apiKeyContextKey, apiKey.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
