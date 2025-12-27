package logger

import (
	"exc6/apperrors"
)

// LogAppError logs an AppError with all its rich context
func LogAppError(err error, level Level) {
	if appErr, ok := err.(*apperrors.AppError); ok {
		WithFields(appErr.LogFields()).log(level, appErr.Message)
	} else {
		WithError(err).log(level, "Unstructured error occurred")
	}
}

// LogAppErrorWithContext logs an AppError with additional context
func LogAppErrorWithContext(err error, level Level, additionalContext map[string]interface{}) {
	if appErr, ok := err.(*apperrors.AppError); ok {
		fields := appErr.LogFields()
		for k, v := range additionalContext {
			fields["extra_"+k] = v
		}
		WithFields(fields).log(level, appErr.Message)
	} else {
		fields := map[string]interface{}{"error": err}
		for k, v := range additionalContext {
			fields[k] = v
		}
		WithFields(fields).log(level, "Unstructured error occurred")
	}
}
