package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
)

var (
	// ErrInvalidFileType indicates unsupported file type
	ErrInvalidFileType = errors.New("invalid file type")
	// ErrFileTooLarge indicates file exceeds size limit
	ErrFileTooLarge = errors.New("file too large")
	// ErrInvalidFilename indicates suspicious filename
	ErrInvalidFilename = errors.New("invalid filename")
)

const (
	MaxFileSize = 5 * 1024 * 1024 // 5MB
)

// AllowedImageExtensions whitelist for profile pictures
var AllowedImageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// AllowedImageMIMETypes whitelist for MIME types
var AllowedImageMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// ValidateImageUpload validates uploaded image files
func ValidateImageUpload(file *multipart.FileHeader) error {
	// Check file size
	if file.Size > MaxFileSize {
		return ErrFileTooLarge
	}

	// Validate MIME type
	contentType := file.Header.Get("Content-Type")
	if !AllowedImageMIMETypes[contentType] {
		return ErrInvalidFileType
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedImageExtensions[ext] {
		return ErrInvalidFileType
	}

	// Check for path traversal attempts
	if strings.Contains(file.Filename, "..") ||
		strings.Contains(file.Filename, "/") ||
		strings.Contains(file.Filename, "\\") {
		return ErrInvalidFilename
	}

	return nil
}

// GenerateSecureFilename creates a cryptographically secure filename
func GenerateSecureFilename(userID string, originalExt string) (string, error) {
	// Generate 16 random bytes
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random filename: %w", err)
	}

	// Clean and validate extension
	ext := strings.ToLower(filepath.Ext(originalExt))
	if !AllowedImageExtensions[ext] {
		ext = ".jpg" // Default fallback
	}

	// Format: userid_randomhex.ext
	filename := fmt.Sprintf("%s_%s%s", userID, hex.EncodeToString(randomBytes), ext)

	return filename, nil
}

// GetSafeUploadPath returns a safe upload path preventing directory traversal
func GetSafeUploadPath(baseDir, filename string) string {
	// Clean the path to prevent directory traversal
	cleanBase := filepath.Clean(baseDir)
	cleanFile := filepath.Base(filename) // Base strips any directory components

	return filepath.Join(cleanBase, cleanFile)
}
