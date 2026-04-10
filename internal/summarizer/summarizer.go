package summarizer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/dividing-by-zaro/flyover/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	openai "github.com/sashabaranov/go-openai"
)

type SummaryResult struct {
	Short string   `json:"short"`
	Long  string   `json:"long"`
	Tags  []string `json:"tags"`
}

type ErrorKind string

const (
	ErrorTransient  ErrorKind = "transient"
	ErrorValidation ErrorKind = "validation"
	ErrorPermanent  ErrorKind = "permanent"
)

type Summarizer struct {
	client  *openai.Client
	model   string
	queries *db.Queries
}

func New(apiKey, model string, queries *db.Queries) *Summarizer {
	client := openai.NewClient(apiKey)
	return &Summarizer{
		client:  client,
		model:   model,
		queries: queries,
	}
}

// NewWithClient creates a summarizer with a custom OpenAI client (for testing).
func NewWithClient(client *openai.Client, model string, queries *db.Queries) *Summarizer {
	return &Summarizer{
		client:  client,
		model:   model,
		queries: queries,
	}
}

const systemPrompt = `You are a content summarizer for a technical reading feed. Given an article, produce a JSON object with exactly these fields:

{
  "short": "A 2-sentence card summary focusing on the author's main point and the single biggest takeaway for the reader.",
  "long": "A 5-6 sentence detailed summary covering the key arguments, methodology, and conclusions. Focus on what makes this piece valuable to a technical reader.",
  "tags": ["tag-1", "tag-2"]
}

Rules:
- "short" must be exactly 2 sentences
- "long" must be 5-6 sentences
- "tags" must be 0-3 short, lowercase, hyphenated topic tags
- Focus on the author's intent and the reader's takeaway
- Do NOT write generic "this post is about..." descriptions
- Return ONLY valid JSON, no markdown or extra text`

const repairPrompt = `The previous response was not valid JSON. Please return ONLY a valid JSON object with these exact fields:
- "short": string (2-sentence summary)
- "long": string (5-6 sentence summary)
- "tags": array of 0-3 lowercase hyphenated strings

Return ONLY the JSON object, nothing else.`

func (s *Summarizer) SummarizePost(ctx context.Context, post db.Post, content string) {
	if content == "" {
		s.markFailed(ctx, post, "empty content", ErrorPermanent)
		return
	}

	// Truncate very long content
	if len(content) > 15000 {
		content = content[:15000]
	}

	var result *SummaryResult
	var lastErr error
	var lastKind ErrorKind

	maxAttempts := 5
	validationRetries := 0

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return
		}

		result, lastKind, lastErr = s.callLLM(ctx, post.Title, content, attempt > 0 && lastKind == ErrorValidation)

		if lastErr == nil && result != nil {
			break
		}

		switch lastKind {
		case ErrorPermanent:
			s.markFailed(ctx, post, lastErr.Error(), ErrorPermanent)
			return

		case ErrorValidation:
			validationRetries++
			if validationRetries >= 2 {
				// Fall back to normal backoff after 2 validation retries
				lastKind = ErrorTransient
			} else {
				continue // Immediate retry with repair prompt
			}

		case ErrorTransient:
			backoff := exponentialBackoffWithJitter(attempt)
			log.Printf("transient error summarizing %s, retrying in %v: %v", post.ID, backoff, lastErr)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
		}
	}

	if result == nil {
		nextAttempt := time.Now().Add(exponentialBackoffWithJitter(int(post.SummaryAttempts)))
		s.queries.UpdatePostSummaryFailed(ctx, db.UpdatePostSummaryFailedParams{
			ID:                   post.ID,
			SummaryError:         pgtype.Text{String: lastErr.Error(), Valid: true},
			SummaryNextAttemptAt: pgtype.Timestamptz{Time: nextAttempt, Valid: true},
			SummaryLastErrorKind: pgtype.Text{String: string(lastKind), Valid: true},
		})
		return
	}

	// Limit tags
	tags := result.Tags
	if len(tags) > 3 {
		tags = tags[:3]
	}

	err := s.queries.UpdatePostSummary(ctx, db.UpdatePostSummaryParams{
		ID:           post.ID,
		SummaryShort: pgtype.Text{String: result.Short, Valid: true},
		SummaryLong:  pgtype.Text{String: result.Long, Valid: true},
		Tags:         tags,
	})
	if err != nil {
		log.Printf("failed to update post summary %s: %v", post.ID, err)
	}
}

func (s *Summarizer) callLLM(ctx context.Context, title, content string, useRepairPrompt bool) (*SummaryResult, ErrorKind, error) {
	userMsg := fmt.Sprintf("Title: %s\n\nContent:\n%s", title, content)

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userMsg},
	}

	if useRepairPrompt {
		messages = append(messages, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleUser, Content: repairPrompt,
		})
	}

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    s.model,
		Messages: messages,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})

	if err != nil {
		kind := classifyError(err)
		return nil, kind, fmt.Errorf("openai api: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, ErrorTransient, fmt.Errorf("no choices in response")
	}

	raw := resp.Choices[0].Message.Content
	raw = strings.TrimSpace(raw)

	var result SummaryResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, ErrorValidation, fmt.Errorf("invalid json: %w", err)
	}

	if result.Short == "" || result.Long == "" {
		return nil, ErrorValidation, fmt.Errorf("empty summary fields")
	}

	return &result, "", nil
}

func (s *Summarizer) markFailed(ctx context.Context, post db.Post, errMsg string, kind ErrorKind) {
	s.queries.UpdatePostSummaryFailed(ctx, db.UpdatePostSummaryFailedParams{
		ID:                   post.ID,
		SummaryError:         pgtype.Text{String: errMsg, Valid: true},
		SummaryLastErrorKind: pgtype.Text{String: string(kind), Valid: true},
	})
}

// SweepPending processes posts with pending summaries that are due for retry.
func (s *Summarizer) SweepPending(ctx context.Context, contentFetcher func(url string) (string, error)) {
	posts, err := s.queries.ListPendingSummaries(ctx)
	if err != nil {
		log.Printf("error listing pending summaries: %v", err)
		return
	}

	for _, post := range posts {
		if ctx.Err() != nil {
			return
		}

		if post.SummaryAttempts >= 5 {
			s.markFailed(ctx, post, "max attempts reached", ErrorPermanent)
			continue
		}

		content := ""
		if contentFetcher != nil {
			fetched, err := contentFetcher(post.Url)
			if err != nil {
				log.Printf("content fetch failed for %s: %v", post.Url, err)
				s.markFailed(ctx, post, "content fetch failed: "+err.Error(), ErrorTransient)
				continue
			}
			content = fetched
		}

		s.SummarizePost(ctx, post, content)

		// Small delay between posts for rate limiting
		time.Sleep(500 * time.Millisecond)
	}
}

func classifyError(err error) ErrorKind {
	errStr := err.Error()
	if strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "connection") {
		return ErrorTransient
	}
	return ErrorTransient // Default to transient for unknown errors
}

func exponentialBackoffWithJitter(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt)) * float64(time.Second)
	jitter := rand.Float64() * float64(time.Second)
	return time.Duration(base) + time.Duration(jitter)
}
