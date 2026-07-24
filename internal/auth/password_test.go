package auth

import "testing"

func TestMinPasswordLen(t *testing.T) {
	t.Parallel()
	if MinPasswordLen != 8 {
		t.Fatalf("expected MinPasswordLen 8, got %d", MinPasswordLen)
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("s3cr3t")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected hash")
	}
	if err := CheckPassword(hash, "s3cr3t"); err != nil {
		t.Fatalf("CheckPassword error: %v", err)
	}
	if err := CheckPassword(hash, "wrong"); err == nil {
		t.Fatalf("expected error for wrong password")
	}
}
