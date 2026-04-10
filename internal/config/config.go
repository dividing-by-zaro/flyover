package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL string
	OpenAIKey   string
	LLMModel    string
	APIKey      string
	BackfillDays int
	PollHour    int
	Port        string
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		LLMModel:     envOr("LLM_MODEL", "gpt-5.4"),
		APIKey:       os.Getenv("API_KEY"),
		BackfillDays: envInt("BACKFILL_DAYS", 60),
		PollHour:     envInt("POLL_HOUR", 3),
		Port:         envOr("PORT", "8080"),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("API_KEY is required")
	}

	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
