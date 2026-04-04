package store

import (
	"errors"
	"unicode"
)

var ErrWeakPassword = errors.New("password must be at least 10 characters and include uppercase, lowercase, and special characters")

func ValidatePasswordComplexity(password string) error {
	if len([]rune(password)) < 10 {
		return ErrWeakPassword
	}

	var hasUpper bool
	var hasLower bool
	var hasSpecial bool

	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsLetter(r), unicode.IsDigit(r):
		default:
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasSpecial {
		return ErrWeakPassword
	}

	return nil
}
