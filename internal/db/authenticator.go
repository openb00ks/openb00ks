package db

import (
	"context"
	"errors"

	"github.com/openb00ks/openb00ks/internal/auth"
)

type Authenticator struct {
	users *UserStore
}

func NewAuthenticator(users *UserStore) *Authenticator {
	return &Authenticator{users: users}
}

func (a *Authenticator) Authenticate(ctx context.Context, email, password string) (string, error) {
	if a == nil || a.users == nil {
		return "", auth.ErrInvalidCredentials
	}
	user, err := a.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", auth.ErrInvalidCredentials
		}
		return "", err
	}
	if err := auth.CheckPassword(user.PasswordHash, password); err != nil {
		return "", auth.ErrInvalidCredentials
	}
	return user.ID, nil
}
