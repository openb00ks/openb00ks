package auth

import (
	"testing"
	"time"
)

func TestGenerateMFASecretProducesBase32Secret(t *testing.T) {
	t.Parallel()

	secret, err := GenerateMFASecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if len(secret) == 0 {
		t.Fatal("expected non-empty secret")
	}
	if !ValidateTOTP(secret, totpAt(secret, time.Unix(1_700_000_000, 0)), time.Unix(1_700_000_000, 0)) {
		t.Fatal("expected generated secret to validate a matching code")
	}
}

func TestBuildMFAProvisioningURI(t *testing.T) {
	t.Parallel()

	uri := BuildMFAProvisioningURI("JBSWY3DPEHPK3PXP", "alex@example.com")
	if uri == "" {
		t.Fatal("expected non-empty uri")
	}
	if want := "otpauth://totp/"; len(uri) < len(want) || uri[:len(want)] != want {
		t.Fatalf("expected otpauth uri, got %q", uri)
	}
}

func TestValidateTOTPAllowsSmallClockSkew(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code := totpAt(secret, now)
	if code == "" {
		t.Fatal("expected code")
	}
	if !ValidateTOTP(secret, code, now.Add(MFAStep)) {
		t.Fatal("expected skewed time to validate")
	}
	if ValidateTOTP(secret, "123456", now) {
		t.Fatal("expected invalid code to fail")
	}
}

func TestValidateTOTPStepReturnsMatchedCounter(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code := totpAt(secret, now)

	step, ok := ValidateTOTPStep(secret, code, now)
	if !ok {
		t.Fatal("expected ValidateTOTPStep to succeed")
	}
	want := now.Unix() / int64(MFAStep.Seconds())
	if step != want {
		t.Fatalf("expected step %d, got %d", want, step)
	}

	// Leeway: code from previous window should match step-1.
	prevCode := totpAt(secret, now.Add(-MFAStep))
	prevStep, ok := ValidateTOTPStep(secret, prevCode, now)
	if !ok {
		t.Fatal("expected leeway code to validate")
	}
	if prevStep != want-1 {
		t.Fatalf("expected leeway step %d, got %d", want-1, prevStep)
	}
}

func TestGenerateRecoveryCodes(t *testing.T) {
	t.Parallel()

	codes, hashes, err := GenerateRecoveryCodes(3)
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	if len(codes) != 3 || len(hashes) != 3 {
		t.Fatalf("expected 3 codes and hashes, got %d/%d", len(codes), len(hashes))
	}
	if codes[0] == codes[1] || hashes[0] == hashes[1] {
		t.Fatal("expected unique recovery codes and hashes")
	}
	if HashRecoveryCode(codes[0]) != hashes[0] {
		t.Fatal("expected hash helper to match generated hash")
	}
}
