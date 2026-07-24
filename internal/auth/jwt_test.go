package auth

import (
	"errors"
	"testing"
	"time"
)

func TestNewTokenAndParseToken(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
	tok, err := NewToken(secret, "user-123", "tenant-456", time.Hour, now)
	if err != nil {
		t.Fatalf("NewToken error: %v", err)
	}

	claims, err := ParseToken(secret, tok, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("ParseToken error: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Fatalf("unexpected user id: %s", claims.UserID)
	}
	if claims.TenantID != "tenant-456" {
		t.Fatalf("unexpected tenant id: %s", claims.TenantID)
	}
	if claims.ExpiresAt == nil {
		t.Fatalf("expected expires at")
	}
}

func TestParseTokenExpired(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
	tok, err := NewToken(secret, "user-123", "tenant-456", time.Minute, now)
	if err != nil {
		t.Fatalf("NewToken error: %v", err)
	}

	_, err = ParseToken(secret, tok, now.Add(2*time.Minute))
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	secret := []byte("test-secret")
	other := []byte("other-secret")
	now := time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
	tok, err := NewToken(secret, "user-123", "tenant-456", time.Hour, now)
	if err != nil {
		t.Fatalf("NewToken error: %v", err)
	}

	_, err = ParseToken(other, tok, now)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseTokenEmpty(t *testing.T) {
	_, err := ParseToken([]byte("secret"), "", time.Now())
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
