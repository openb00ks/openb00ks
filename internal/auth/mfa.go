package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	MFAIssuer         = "Open B00KS"
	MFADigits         = 6
	MFAStep           = 30 * time.Second
	MFALeeway         = 1
	MFATokenPurpose   = "mfa_login"
	MFATokenTTL       = 10 * time.Minute
	mfaSecretLength   = 20
	mfaRecoveryLength = 10
	mfaRecoveryChars  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	mfaAlgorithmParam = "SHA1"
)

func GenerateMFASecret() (string, error) {
	buf := make([]byte, mfaSecretLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	return encoder.EncodeToString(buf), nil
}

func BuildMFAProvisioningURI(secret, accountName string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", MFAIssuer)
	v.Set("algorithm", mfaAlgorithmParam)
	v.Set("digits", strconv.Itoa(MFADigits))
	v.Set("period", strconv.Itoa(int(MFAStep.Seconds())))
	label := url.PathEscape(MFAIssuer + ":" + strings.TrimSpace(accountName))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, v.Encode())
}

func ValidateTOTP(secret, code string, now time.Time) bool {
	_, ok := ValidateTOTPStep(secret, code, now)
	return ok
}

// ValidateTOTPStep validates the code and returns the TOTP step counter that
// matched.  Callers that prevent replay attacks should record the step and
// reject reuse.  step = Unix()/30 adjusted for leeway.
func ValidateTOTPStep(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.TrimSpace(code)
	if len(code) != MFADigits {
		return 0, false
	}
	base := now.Unix() / int64(MFAStep.Seconds())
	for offset := int64(-MFALeeway); offset <= int64(MFALeeway); offset++ {
		t := now.Add(time.Duration(offset) * MFAStep)
		if totpAt(secret, t) == code {
			return base + offset, true
		}
	}
	return 0, false
}

func GenerateTOTPCode(secret string, at time.Time) string {
	return totpAt(secret, at)
}

func GenerateRecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		count = mfaRecoveryLength
	}
	codes := make([]string, 0, count)
	hashes := make([]string, 0, count)
	for range make([]struct{}, count) {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes, nil
}

func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func totpAt(secret string, at time.Time) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := encoder.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	counter := uint64(at.Unix() / int64(MFAStep.Seconds()))
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	if len(sum) < 20 {
		return ""
	}
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < MFADigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", MFADigits, binCode%mod)
}

func generateRecoveryCode() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, 0, 10)
	for _, b := range buf {
		out = append(out, mfaRecoveryChars[int(b)%len(mfaRecoveryChars)])
	}
	return string(out), nil
}
