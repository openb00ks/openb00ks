package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const MinPasswordLen = 8

var ErrPasswordTooShort = errors.New("password must be at least 8 characters")

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
