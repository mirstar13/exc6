package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"exc6/apperrors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

const (
	MaxFileSize       = 5 * 1024 * 1024 // 5MB
	MaxImageDimension = 4096            // Max width/height in pixels
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

// MagicBytes defines the first bytes of valid image formats
var MagicBytes = map[string][]byte{
	"image/jpeg": {0xFF, 0xD8, 0xFF},
	"image/png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"image/gif":  {0x47, 0x49, 0x46, 0x38}, // GIF87a or GIF89a
	"image/webp": {0x52, 0x49, 0x46, 0x46}, // RIFF (WebP container)
}

// ValidationResult contains detailed validation information
type ValidationResult struct {
	Valid        bool
	DetectedMIME string
	DeclaredMIME string
	Width        int
	Height       int
	FileSize     int64
	Errors       []string
}

// ValidateImageUploadStrict performs comprehensive image validation
func ValidateImageUploadStrict(fileHeader *multipart.FileHeader) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:        true,
		DeclaredMIME: fileHeader.Header.Get("Content-Type"),
		FileSize:     fileHeader.Size,
	}

	// 1. Check file size
	if fileHeader.Size > MaxFileSize {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("file too large: %d bytes (max: %d)", fileHeader.Size, MaxFileSize))
		return result, apperrors.NewFileTooLarge(MaxFileSize)
	}

	if fileHeader.Size == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "file is empty")
		return result, apperrors.NewValidationError("Empty file uploaded")
	}

	// 2. Validate declared MIME type
	if !AllowedImageMIMETypes[result.DeclaredMIME] {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid MIME type: %s", result.DeclaredMIME))
		allowed := make([]string, 0, len(AllowedImageMIMETypes))
		for mime := range AllowedImageMIMETypes {
			allowed = append(allowed, mime)
		}
		return result, apperrors.NewInvalidFileType(allowed)
	}

	// 3. Validate extension
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !AllowedImageExtensions[ext] {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid extension: %s", ext))
		allowed := make([]string, 0, len(AllowedImageExtensions))
		for e := range AllowedImageExtensions {
			allowed = append(allowed, e)
		}
		return result, apperrors.NewInvalidFileType(allowed)
	}

	// 4. Check for path traversal attempts
	if strings.Contains(fileHeader.Filename, "..") ||
		strings.Contains(fileHeader.Filename, "/") ||
		strings.Contains(fileHeader.Filename, "\\") {
		result.Valid = false
		result.Errors = append(result.Errors, "filename contains invalid characters")
		return result, apperrors.New(
			apperrors.ErrCodeInvalidFilename,
			"Filename contains invalid characters",
			400,
		)
	}

	// 5. Open file for content inspection
	file, err := fileHeader.Open()
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to open file: %v", err))
		return result, apperrors.NewInternalError("Failed to open uploaded file").WithInternal(err)
	}
	defer file.Close()

	// 6. Read file content into buffer for multiple checks
	fileContent, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to read file: %v", err))
		return result, apperrors.NewInternalError("Failed to read uploaded file").WithInternal(err)
	}

	// 7. Detect actual MIME type from content (magic bytes)
	result.DetectedMIME = http.DetectContentType(fileContent)

	// 8. Verify magic bytes match expected format
	if !validateMagicBytes(fileContent, result.DeclaredMIME) {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("magic bytes mismatch: declared=%s, detected=%s",
				result.DeclaredMIME, result.DetectedMIME))
		return result, apperrors.NewValidationError(
			"File content does not match declared type. Possible file manipulation detected.",
		)
	}

	// 9. Validate MIME type consistency
	if !isCompatibleMIME(result.DetectedMIME, result.DeclaredMIME) {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("MIME type mismatch: declared=%s, detected=%s",
				result.DeclaredMIME, result.DetectedMIME))
		return result, apperrors.NewValidationError(
			"File type mismatch detected",
		)
	}

	// 10. Decode image to validate it's a real image and get dimensions
	reader := bytes.NewReader(fileContent)
	imgConfig, format, err := image.DecodeConfig(reader)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid image format: %v", err))
		return result, apperrors.NewValidationError("File is not a valid image or is corrupted")
	}

	result.Width = imgConfig.Width
	result.Height = imgConfig.Height

	// 11. Check image dimensions (prevent memory exhaustion attacks)
	if imgConfig.Width > MaxImageDimension || imgConfig.Height > MaxImageDimension {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("image too large: %dx%d (max: %dx%d)",
				imgConfig.Width, imgConfig.Height, MaxImageDimension, MaxImageDimension))
		return result, apperrors.NewValidationError(
			fmt.Sprintf("Image dimensions exceed maximum allowed size (%dx%d)",
				MaxImageDimension, MaxImageDimension),
		)
	}

	// 12. Validate format matches extension
	expectedFormat := getFormatFromExtension(ext)
	if format != expectedFormat && !(format == "jpeg" && expectedFormat == "jpg") {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("format mismatch: extension=%s, actual=%s", ext, format))
		return result, apperrors.NewValidationError(
			"File extension does not match actual image format",
		)
	}

	return result, nil
}

// validateMagicBytes checks if file starts with expected magic bytes
func validateMagicBytes(content []byte, mimeType string) bool {
	expectedMagic, exists := MagicBytes[mimeType]
	if !exists {
		return false
	}

	if len(content) < len(expectedMagic) {
		return false
	}

	// Special case for WebP (need to check WEBP marker at offset 8)
	if mimeType == "image/webp" {
		if len(content) < 12 {
			return false
		}
		return bytes.Equal(content[0:4], expectedMagic) &&
			bytes.Equal(content[8:12], []byte("WEBP"))
	}

	return bytes.Equal(content[0:len(expectedMagic)], expectedMagic)
}

// isCompatibleMIME checks if detected MIME is compatible with declared MIME
func isCompatibleMIME(detected, declared string) bool {
	// Exact match
	if detected == declared {
		return true
	}

	// Handle JPEG variations
	if (detected == "image/jpeg" || detected == "image/jpg") &&
		(declared == "image/jpeg" || declared == "image/jpg") {
		return true
	}

	return false
}

// getFormatFromExtension returns expected image format from file extension
func getFormatFromExtension(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".png":
		return "png"
	case ".gif":
		return "gif"
	case ".webp":
		return "webp"
	default:
		return ""
	}
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

	// Format: userid_timestamp_randomhex.ext
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	filename := fmt.Sprintf("%s_%s_%s%s",
		userID,
		timestamp,
		hex.EncodeToString(randomBytes),
		ext)

	return filename, nil
}

// GetSafeUploadPath returns a safe upload path preventing directory traversal
func GetSafeUploadPath(baseDir, filename string) string {
	// Clean the path to prevent directory traversal
	cleanBase := filepath.Clean(baseDir)
	cleanFile := filepath.Base(filename) // Base strips any directory components

	return filepath.Join(cleanBase, cleanFile)
}

// SanitizeFilename removes any potentially dangerous characters
func SanitizeFilename(filename string) string {
	// Remove path separators
	filename = strings.ReplaceAll(filename, "/", "")
	filename = strings.ReplaceAll(filename, "\\", "")
	filename = strings.ReplaceAll(filename, "..", "")

	// Remove null bytes
	filename = strings.ReplaceAll(filename, "\x00", "")

	// Limit length
	if len(filename) > 255 {
		ext := filepath.Ext(filename)
		filename = filename[:255-len(ext)] + ext
	}

	return filename
}
