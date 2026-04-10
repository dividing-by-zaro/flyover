package poller

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/go-shiori/go-readability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mmcdole/gofeed"
)

type Poller struct {
	Queries      *db.Queries
	Summarize    func(ctx context.Context, post db.Post, content string)
	BackfillDays int
	MaxWorkers   int
}

func New(queries *db.Queries, backfillDays int) *Poller {
	return &Poller{
		Queries:      queries,
		BackfillDays: backfillDays,
		MaxWorkers:   5,
	}
}

func (p *Poller) PollAll(ctx context.Context) error {
	sources, err := p.Queries.ListActiveRSSSources(ctx)
	if err != nil {
		return fmt.Errorf("list active RSS sources: %w", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, p.MaxWorkers)

	for _, source := range sources {
		wg.Add(1)
		sem <- struct{}{}
		go func(src db.Source) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := p.pollSource(ctx, src, 0); err != nil {
				log.Printf("error polling source %s (%s): %v", src.Name, src.ID, err)
			}
		}(source)
	}

	wg.Wait()
	return nil
}

func (p *Poller) Backfill(ctx context.Context) error {
	count, err := p.Queries.CountPosts(ctx)
	if err != nil {
		return fmt.Errorf("count posts: %w", err)
	}
	if count > 0 {
		log.Println("posts already exist, skipping backfill")
		return nil
	}

	log.Printf("starting initial backfill for last %d days", p.BackfillDays)
	sources, err := p.Queries.ListActiveRSSSources(ctx)
	if err != nil {
		return fmt.Errorf("list sources for backfill: %w", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, p.MaxWorkers)

	for _, source := range sources {
		wg.Add(1)
		sem <- struct{}{}
		go func(src db.Source) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := p.pollSource(ctx, src, p.BackfillDays); err != nil {
				log.Printf("error backfilling source %s: %v", src.Name, err)
			}
		}(source)
	}

	wg.Wait()
	log.Println("backfill complete")
	return nil
}

func (p *Poller) pollSource(ctx context.Context, source db.Source, backfillDays int) error {
	feedURL := source.FeedUrl.String
	if !source.FeedUrl.Valid || feedURL == "" {
		if source.SiteUrl.Valid && source.SiteUrl.String != "" {
			discovered, err := discoverFeedURL(ctx, source.SiteUrl.String)
			if err != nil {
				log.Printf("feed discovery failed for %s, trying readability scrape", source.Name)
				return p.scrapeSite(ctx, source)
			}
			feedURL = discovered
			_ = p.Queries.UpdateSourceFeedURL(ctx, db.UpdateSourceFeedURLParams{
				FeedUrl: pgtype.Text{String: feedURL, Valid: true},
				ID:      source.ID,
			})
		} else {
			return fmt.Errorf("no feed_url or site_url for source %s", source.Name)
		}
	}

	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return fmt.Errorf("parse feed %s: %w", feedURL, err)
	}

	cutoff := time.Time{}
	if backfillDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -backfillDays)
	}

	for _, item := range feed.Items {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		postURL := item.Link
		if postURL == "" {
			continue
		}

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		if !cutoff.IsZero() && publishedAt.Before(cutoff) {
			continue
		}

		exists, err := p.Queries.PostExistsByURL(ctx, postURL)
		if err != nil {
			log.Printf("error checking URL existence: %v", err)
			continue
		}
		if exists {
			continue
		}

		title := item.Title
		if title == "" {
			title = postURL
		}

		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}

		var imageURL string
		if item.Image != nil {
			imageURL = item.Image.URL
		}

		post, err := p.Queries.CreatePost(ctx, db.CreatePostParams{
			SourceID:      source.ID,
			Title:         title,
			Url:           postURL,
			Author:        pgtype.Text{String: author, Valid: author != ""},
			SummaryStatus: "pending",
			Tags:          []string{},
			ImageUrl:      pgtype.Text{String: imageURL, Valid: imageURL != ""},
			PublishedAt:   publishedAt,
		})
		if err != nil {
			log.Printf("error creating post %s: %v", postURL, err)
			continue
		}

		// Try to fetch full content for summarization
		content := extractContent(item)
		if content == "" {
			fetched, err := fetchArticleContent(ctx, postURL)
			if err != nil {
				log.Printf("content extraction failed for %s: %v", postURL, err)
			} else {
				content = fetched
			}
		}

		if content != "" && p.Summarize != nil {
			p.Summarize(ctx, post, content)
		}
	}

	_ = p.Queries.UpdateSourceLastPolled(ctx, source.ID)
	return nil
}

func (p *Poller) scrapeSite(ctx context.Context, source db.Source) error {
	content, err := fetchArticleContent(ctx, source.SiteUrl.String)
	if err != nil {
		return fmt.Errorf("scrape %s: %w", source.SiteUrl.String, err)
	}

	if content == "" {
		return fmt.Errorf("no content extracted from %s", source.SiteUrl.String)
	}

	exists, _ := p.Queries.PostExistsByURL(ctx, source.SiteUrl.String)
	if exists {
		return nil
	}

	post, err := p.Queries.CreatePost(ctx, db.CreatePostParams{
		SourceID:      source.ID,
		Title:         source.Name,
		Url:           source.SiteUrl.String,
		SummaryStatus: "pending",
		Tags:          []string{},
		PublishedAt:   time.Now(),
	})
	if err != nil {
		return fmt.Errorf("create scraped post: %w", err)
	}

	if p.Summarize != nil {
		p.Summarize(ctx, post, content)
	}

	return nil
}

func extractContent(item *gofeed.Item) string {
	if item.Content != "" {
		return item.Content
	}
	return item.Description
}

func fetchArticleContent(ctx context.Context, url string) (string, error) {
	article, err := readability.FromURL(url, 30*time.Second)
	if err != nil {
		return "", err
	}
	return article.TextContent, nil
}

func discoverFeedURL(ctx context.Context, siteURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", siteURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Flyover/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return "", err
	}

	html := string(body)

	// Look for RSS/Atom link tags
	for _, linkType := range []string{
		`application/rss+xml`,
		`application/atom+xml`,
		`application/feed+json`,
	} {
		idx := strings.Index(html, linkType)
		if idx == -1 {
			continue
		}
		// Find the surrounding <link> tag
		start := strings.LastIndex(html[:idx], "<link")
		if start == -1 {
			continue
		}
		end := strings.Index(html[start:], ">")
		if end == -1 {
			continue
		}
		linkTag := html[start : start+end+1]

		hrefIdx := strings.Index(linkTag, `href="`)
		if hrefIdx == -1 {
			hrefIdx = strings.Index(linkTag, `href='`)
			if hrefIdx == -1 {
				continue
			}
		}
		hrefStart := hrefIdx + 6
		quote := linkTag[hrefIdx+5]
		hrefEnd := strings.IndexByte(linkTag[hrefStart:], quote)
		if hrefEnd == -1 {
			continue
		}
		feedURL := linkTag[hrefStart : hrefStart+hrefEnd]

		// Make absolute URL if relative
		if strings.HasPrefix(feedURL, "/") {
			feedURL = strings.TrimRight(siteURL, "/") + feedURL
		}

		return feedURL, nil
	}

	return "", fmt.Errorf("no feed link found on %s", siteURL)
}

// SeedSources seeds initial sources if none exist.
func SeedSources(ctx context.Context, queries *db.Queries) error {
	sources, err := queries.ListSources(ctx)
	if err != nil {
		return err
	}
	if len(sources) > 0 {
		return nil
	}

	type seed struct {
		name    string
		siteURL string
		feedURL string
	}

	seeds := []seed{
		{"Lilian Weng", "https://lilianweng.github.io/", "https://lilianweng.github.io/index.xml"},
		{"swyx", "https://www.swyx.io/", "https://www.swyx.io/rss.xml"},
		{"Eugene Yan", "https://eugeneyan.com/", "https://eugeneyan.com/rss/"},
		{"Tania Rascia", "https://www.taniarascia.com/", "https://www.taniarascia.com/rss.xml"},
	}

	for _, s := range seeds {
		_, err := queries.CreateSource(ctx, db.CreateSourceParams{
			Kind:    "rss",
			Name:    s.name,
			SiteUrl: pgtype.Text{String: s.siteURL, Valid: true},
			FeedUrl: pgtype.Text{String: s.feedURL, Valid: true},
		})
		if err != nil {
			log.Printf("failed to seed source %s: %v", s.name, err)
		}
	}

	log.Println("seeded initial sources")
	return nil
}

// IgnoreUUID is a helper to suppress unused import errors in tests.
var _ = uuid.New
