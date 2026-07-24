package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	_ = os.Unsetenv("API_ADDR")
	_ = os.Unsetenv("DATABASE_URL")
	_ = os.Unsetenv("JWT_SECRET")
	_ = os.Unsetenv("RECEIPT_STORAGE")
	_ = os.Unsetenv("RECEIPT_LOCAL_DIR")
	_ = os.Unsetenv("RECEIPT_MAX_BYTES")
	_ = os.Unsetenv("AI_PROVIDER")
	_ = os.Unsetenv("OPENAI_API_KEY")
	_ = os.Unsetenv("OPENAI_MODEL")
	_ = os.Unsetenv("JWT_TTL_SECONDS")
	_ = os.Unsetenv("REFRESH_TTL_SECONDS")
	_ = os.Unsetenv("CORS_ALLOWED_ORIGINS")
	_ = os.Unsetenv("LOG_LEVEL")
	_ = os.Unsetenv("LOG_FORMAT")
	_ = os.Unsetenv("AI_INPUT_CENTS_PER_1K_TOKENS")
	_ = os.Unsetenv("AI_OUTPUT_CENTS_PER_1K_TOKENS")
	_ = os.Unsetenv("ENABLE_PUBLIC_REGISTRATION")
	_ = os.Unsetenv("SEARCH_PROVIDER")
	_ = os.Unsetenv("TYPESENSE_URL")
	_ = os.Unsetenv("TYPESENSE_API_KEY")
	_ = os.Unsetenv("TYPESENSE_COLLECTION_PREFIX")

	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Fatalf("expected default addr, got %s", cfg.Addr)
	}
	if cfg.ReceiptMaxBytes != 10*1024*1024 {
		t.Fatalf("expected default receipt max bytes")
	}
	if cfg.JWTTTLSeconds != 86400 {
		t.Fatalf("expected default jwt ttl seconds")
	}
	if cfg.RefreshTTLSeconds != 2592000 {
		t.Fatalf("expected default refresh ttl seconds")
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:5173" {
		t.Fatalf("expected default cors origin")
	}
	if cfg.LogLevel != "info" || cfg.LogFormat != "json" {
		t.Fatalf("expected default log settings")
	}
	if cfg.AIInputCentsPer1KTokens != 0 || cfg.AIOutputCentsPer1KTokens != 0 {
		t.Fatalf("expected default AI pricing to be zero")
	}
	if cfg.EnablePublicRegistration {
		t.Fatalf("expected public registration disabled by default")
	}
	if cfg.SearchProvider != "none" || cfg.TypesenseCollectionPrefix != "openb00ks" {
		t.Fatalf("expected search defaults, got provider=%q prefix=%q", cfg.SearchProvider, cfg.TypesenseCollectionPrefix)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("API_ADDR", ":9999")
	t.Setenv("RECEIPT_MAX_BYTES", "123")
	t.Setenv("JWT_TTL_SECONDS", "3600")
	t.Setenv("REFRESH_TTL_SECONDS", "7200")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com, http://localhost:5173")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("AI_INPUT_CENTS_PER_1K_TOKENS", "1")
	t.Setenv("AI_OUTPUT_CENTS_PER_1K_TOKENS", "2")
	t.Setenv("ENABLE_PUBLIC_REGISTRATION", "true")
	t.Setenv("SEARCH_PROVIDER", "typesense")
	t.Setenv("TYPESENSE_URL", "http://localhost:8108")
	t.Setenv("TYPESENSE_API_KEY", "xyz")
	t.Setenv("TYPESENSE_COLLECTION_PREFIX", "devbooks")

	cfg := Load()
	if cfg.Addr != ":9999" {
		t.Fatalf("expected addr override")
	}
	if cfg.ReceiptMaxBytes != 123 {
		t.Fatalf("expected receipt max bytes override")
	}
	if cfg.JWTTTLSeconds != 3600 {
		t.Fatalf("expected jwt ttl override")
	}
	if cfg.RefreshTTLSeconds != 7200 {
		t.Fatalf("expected refresh ttl override")
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[0] != "https://example.com" || cfg.CORSAllowedOrigins[1] != "http://localhost:5173" {
		t.Fatalf("expected cors origins override")
	}
	if cfg.LogLevel != "debug" || cfg.LogFormat != "text" {
		t.Fatalf("expected log settings override")
	}
	if cfg.AIInputCentsPer1KTokens != 1 || cfg.AIOutputCentsPer1KTokens != 2 {
		t.Fatalf("expected AI pricing override")
	}
	if !cfg.EnablePublicRegistration {
		t.Fatalf("expected public registration override")
	}
	if cfg.SearchProvider != "typesense" || cfg.TypesenseURL != "http://localhost:8108" || cfg.TypesenseAPIKey != "xyz" || cfg.TypesenseCollectionPrefix != "devbooks" {
		t.Fatalf("expected search overrides, got %#v", cfg)
	}
}
