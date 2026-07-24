package auth

import (
	"errors"
	"testing"
	"time"
)

const testSecret = "test-secret-32-bytes-exactly----!" // 32 bytes

func TestTokenServiceIssueAndParse(t *testing.T) {
	now := time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)
	svc, err := NewTokenService(testSecret, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewTokenService error: %v", err)
	}

	tok, err := svc.Issue("user-abc", "tenant-xyz", time.Hour)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	claims, err := svc.Parse(tok)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if claims.UserID != "user-abc" {
		t.Fatalf("unexpected user id: %s", claims.UserID)
	}
	if claims.TenantID != "tenant-xyz" {
		t.Fatalf("unexpected tenant id: %s", claims.TenantID)
	}
}

func TestTokenServiceMissingSecret(t *testing.T) {
	_, err := NewTokenService("", time.Now)
	if !errors.Is(err, ErrMissingSecret) {
		t.Fatalf("expected ErrMissingSecret, got %v", err)
	}
}

func TestTokenServiceSecretTooShort(t *testing.T) {
	_, err := NewTokenService("tooshort", time.Now)
	if !errors.Is(err, ErrSecretTooShort) {
		t.Fatalf("expected ErrSecretTooShort, got %v", err)
	}
}
