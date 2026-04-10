.PHONY: test build dev frontend clean

# Run tests sequentially (packages share a test database)
test:
	go test ./... -count=1 -p 1

# Run tests with verbose output
test-v:
	go test ./... -count=1 -p 1 -v

# Build the Go binary (requires frontend to be built first)
build: frontend
	cp -r frontend/dist internal/embed/frontend/dist
	cp migrations/*.sql internal/embed/migrations/sql/
	go build -o flyover ./cmd/server

# Build frontend
frontend:
	cd frontend && npm ci && npm run build

# Start dev server (requires postgres running)
dev:
	go run ./cmd/server

# Clean build artifacts
clean:
	rm -f flyover
	rm -rf frontend/dist internal/embed/frontend/dist
