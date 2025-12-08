package utils

import (
	"exc6/apperrors"

	"golang.org/x/crypto/bcrypt"
)

func ValidatePasswordStrength(password string) *apperrors.AppError {
	if len(password) < 8 {
		return apperrors.NewWeakPassword("Password must be at least 8 characters long")
	}
	// Additional strength checks can be added here (e.g., uppercase, numbers, symbols)
	return nil
}

func HashPassword(password string) (string, *apperrors.AppError) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal, "Failed to hash password", 500).WithInternal(err)
	}
	return string(hashed), nil
}
