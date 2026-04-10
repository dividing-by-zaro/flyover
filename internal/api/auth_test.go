package api

import (
	"testing"
)

func TestHashAPIKey(t *testing.T) {
	key := "test-api-key-12345"
	hash1 := HashAPIKey(key)
	hash2 := HashAPIKey(key)

	if hash1 != hash2 {
		t.Error("same key should produce same hash")
	}

	if hash1 == "" {
		t.Error("hash should not be empty")
	}

	differentHash := HashAPIKey("different-key")
	if hash1 == differentHash {
		t.Error("different keys should produce different hashes")
	}
}

func TestHashAPIKey_Length(t *testing.T) {
	hash := HashAPIKey("any-key")
	if len(hash) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars, got %d", len(hash))
	}
}
