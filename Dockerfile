# Build frontend
FROM node:20 AS frontend
WORKDIR /app/frontend
COPY frontend/ .
RUN npm ci && npm run build

# Build Go binary
FROM golang:1.22 AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -o /flyover ./cmd/server

# Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=backend /flyover /flyover
EXPOSE 8080
CMD ["/flyover"]
