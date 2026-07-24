package auth

import (
	"context"
	"errors"
	"time"
)

var ErrMissingSecret = errors.New("missing jwt secret")
var ErrSecretTooShort = errors.New("jwt secret must be at least 32 bytes")
var ErrInvalidCredentials = errors.New("invalid credentials")

type TokenService struct {
	secret []byte
	now    func() time.Time
}

type Authenticator interface {
	Authenticate(ctx context.Context, email, password string) (string, error)
}

func NewTokenService(secret string, now func() time.Time) (*TokenService, error) {
	if secret == "" {
		return nil, ErrMissingSecret
	}
	if len(secret) < 32 {
		return nil, ErrSecretTooShort
	}
	if now == nil {
		now = time.Now
	}
	return &TokenService{secret: []byte(secret), now: now}, nil
}

func (s *TokenService) Issue(userID, tenantID string, ttl time.Duration) (string, error) {
	return NewToken(s.secret, userID, tenantID, ttl, s.now())
}

func (s *TokenService) Parse(tokenString string) (Claims, error) {
	return ParseToken(s.secret, tokenString, s.now())
}
