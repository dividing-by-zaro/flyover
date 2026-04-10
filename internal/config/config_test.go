package config

import (
	"os"
	"testing"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	os.Clearenv()
	os.Setenv("API_KEY", "test-key")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoad_RequiresAPIKey(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when API_KEY is missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("API_KEY", "test-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.LLMModel != "gpt-5.4" {
		t.Errorf("expected default model gpt-5.4, got %s", cfg.LLMModel)
	}
	if cfg.BackfillDays != 60 {
		t.Errorf("expected default backfill 60, got %d", cfg.BackfillDays)
	}
	if cfg.PollHour != 3 {
		t.Errorf("expected default poll hour 3, got %d", cfg.PollHour)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("API_KEY", "test-key")
	os.Setenv("PORT", "9090")
	os.Setenv("LLM_MODEL", "gpt-4o")
	os.Setenv("BACKFILL_DAYS", "30")
	os.Setenv("POLL_HOUR", "6")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Port)
	}
	if cfg.LLMModel != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", cfg.LLMModel)
	}
	if cfg.BackfillDays != 30 {
		t.Errorf("expected backfill 30, got %d", cfg.BackfillDays)
	}
	if cfg.PollHour != 6 {
		t.Errorf("expected poll hour 6, got %d", cfg.PollHour)
	}
}
