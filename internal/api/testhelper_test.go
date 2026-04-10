package api

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"

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
		t.Skipf("skipping integration test: cannot connect to postgres: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping integration test: cannot ping postgres: %v", err)
	}

	// Run migrations using file source
	m, err := migrate.New("file://../../migrations", testDBURL())
	if err != nil {
		t.Fatalf("failed to create migrator: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Logf("migration warning: %v", err)
	}

	// Clean tables for test isolation (CASCADE handles FK ordering)
	_, err = pool.Exec(ctx, "TRUNCATE posts, sources, api_keys CASCADE")
	if err != nil {
		t.Fatalf("failed to clean tables: %v", err)
	}

	return db.New(pool)
}

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	queries := setupTestDB(t)
	return NewServer(queries, nil)
}

func seedAPIKey(t *testing.T, queries *db.Queries, rawKey string) {
	t.Helper()
	hash := HashAPIKey(rawKey)
	_, err := queries.CreateAPIKey(context.Background(), db.CreateAPIKeyParams{
		KeyHash: hash,
		Name:    fmt.Sprintf("test-key-%s", rawKey[:8]),
	})
	if err != nil {
		t.Fatalf("failed to seed API key: %v", err)
	}
}
