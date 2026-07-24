package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidChallenge = errors.New("invalid challenge")

type ChallengeClaims struct {
	UserID   string `json:"sub"`
	TenantID string `json:"tenant_id"`
	Purpose  string `json:"purpose"`
	jwt.RegisteredClaims
}

func (s *TokenService) IssueChallenge(userID, tenantID, purpose string, ttl time.Duration) (string, error) {
	claims := ChallengeClaims{
		UserID:   userID,
		TenantID: tenantID,
		Purpose:  purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(s.now()),
			ExpiresAt: jwt.NewNumericDate(s.now().Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *TokenService) ParseChallenge(tokenString, purpose string) (ChallengeClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(s.now),
	)
	parsed, err := parser.ParseWithClaims(tokenString, &ChallengeClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return ChallengeClaims{}, ErrExpiredToken
		}
		return ChallengeClaims{}, ErrInvalidChallenge
	}
	claims, ok := parsed.Claims.(*ChallengeClaims)
	if !ok || !parsed.Valid {
		return ChallengeClaims{}, ErrInvalidChallenge
	}
	if claims.Purpose != purpose {
		return ChallengeClaims{}, ErrInvalidChallenge
	}
	if claims.UserID == "" || claims.TenantID == "" {
		return ChallengeClaims{}, ErrInvalidChallenge
	}
	return *claims, nil
}
