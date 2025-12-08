package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"exc6/apperrors"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
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
		return apperrors.NewFileTooLarge(MaxFileSize)
	}

	// Validate MIME type
	contentType := file.Header.Get("Content-Type")
	if !AllowedImageMIMETypes[contentType] {
		allowed := make([]string, 0, len(AllowedImageMIMETypes))
		for mime := range AllowedImageMIMETypes {
			allowed = append(allowed, mime)
		}
		return apperrors.NewInvalidFileType(allowed)
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedImageExtensions[ext] {
		allowed := make([]string, 0, len(AllowedImageExtensions))
		for ext := range AllowedImageExtensions {
			allowed = append(allowed, ext)
		}
		return apperrors.NewInvalidFileType(allowed)
	}

	// Check for path traversal attempts
	if strings.Contains(file.Filename, "..") ||
		strings.Contains(file.Filename, "/") ||
		strings.Contains(file.Filename, "\\") {
		return apperrors.New(
			apperrors.ErrCodeInvalidFilename,
			"Filename contains invalid characters",
			400,
		)
	}

	return nil
}

// GenerateSecureFilename creates a cryptographically secure filename
func GenerateSecureFilename(userID string, originalExt string) (string, error) {
	// Generate 16 random bytes
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", apperrors.NewInternalError("Failed to generate secure filename").WithInternal(err)
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

func validateImageMagicBytes(file multipart.File) error {
	header := make([]byte, 512)
	file.Read(header)
	file.Seek(0, 0)

	contentType := http.DetectContentType(header)
	if !AllowedImageMIMETypes[contentType] {
		return errors.New("invalid image format")
	}
	return nil
}
