package utils

import (
	"exc6/apperrors"
	"regexp"
)

var (
	usernameRegex  = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	groupNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\s]+$`)
)

// ValidateUsername checks if the username meets security requirements
func ValidateUsername(username string) *apperrors.AppError {
	if len(username) < 3 {
		return apperrors.NewValidationError("Username must be at least 3 characters long")
	}

	if len(username) > 30 {
		return apperrors.NewValidationError("Username cannot exceed 30 characters")
	}

	if !usernameRegex.MatchString(username) {
		return apperrors.NewValidationError("Username can only contain letters, numbers, underscores, and hyphens")
	}

	return nil
}

// ValidateGroupName checks if the group name meets security requirements
func ValidateGroupName(name string) *apperrors.AppError {
	if len(name) < 3 {
		return apperrors.NewValidationError("Group name must be at least 3 characters long")
	}

	if len(name) > 50 {
		return apperrors.NewValidationError("Group name cannot exceed 50 characters")
	}

	if !groupNameRegex.MatchString(name) {
		return apperrors.NewValidationError("Group name can only contain letters, numbers, spaces, underscores, and hyphens")
	}

	return nil
}
