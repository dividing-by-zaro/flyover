package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dividing-by-zaro/flyover/internal/api"
	"github.com/dividing-by-zaro/flyover/internal/config"
	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/dividing-by-zaro/flyover/internal/embed/frontend"
	"github.com/dividing-by-zaro/flyover/internal/embed/migrations"
	"github.com/dividing-by-zaro/flyover/internal/poller"
	"github.com/dividing-by-zaro/flyover/internal/summarizer"
	"github.com/go-shiori/go-readability"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

func main() {
	log.SetOutput(os.Stdout)
	log.Println("flyover starting...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database ping: %v", err)
	}
	log.Println("connected to database")

	// Run migrations
	migFS := migrations.FS()
	if err := db.RunMigrations(cfg.DatabaseURL, migFS); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Println("migrations complete")

	queries := db.New(pool)

	// Seed API key on first boot
	keyCount, err := queries.CountAPIKeys(ctx)
	if err != nil {
		log.Fatalf("count api keys: %v", err)
	}
	if keyCount == 0 {
		hash := api.HashAPIKey(cfg.APIKey)
		_, err := queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
			KeyHash: hash,
			Name:    "primary-admin",
		})
		if err != nil {
			log.Fatalf("seed api key: %v", err)
		}
		log.Println("seeded admin API key")
	}

	// Seed initial sources
	if err := poller.SeedSources(ctx, queries); err != nil {
		log.Printf("seed sources warning: %v", err)
	}

	// Summarizer
	var sum *summarizer.Summarizer
	if cfg.OpenAIKey != "" {
		sum = summarizer.New(cfg.OpenAIKey, cfg.LLMModel, queries)
	}

	// Poller
	p := poller.New(queries, cfg.BackfillDays)
	if sum != nil {
		p.Summarize = func(ctx context.Context, post db.Post, content string) {
			sum.SummarizePost(ctx, post, content)
		}
	}

	// Initial backfill (background)
	go func() {
		if err := p.Backfill(ctx); err != nil {
			log.Printf("backfill error: %v", err)
		}
	}()

	// Cron scheduler
	c := cron.New()
	cronSpec := fmt.Sprintf("0 %d * * *", cfg.PollHour)
	c.AddFunc(cronSpec, func() {
		log.Println("starting daily poll...")
		if err := p.PollAll(ctx); err != nil {
			log.Printf("daily poll error: %v", err)
		}
		if sum != nil {
			contentFetcher := func(url string) (string, error) {
				article, err := readability.FromURL(url, 30*time.Second)
				if err != nil {
					return "", err
				}
				return article.TextContent, nil
			}
			sum.SweepPending(ctx, contentFetcher)
		}
		log.Println("daily poll complete")
	})
	c.Start()
	defer c.Stop()

	// Frontend
	feFS := frontend.FS()

	// HTTP server
	srv := api.NewServer(queries, feFS)
	httpSrv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: srv.Router,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on :%s", cfg.Port)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
