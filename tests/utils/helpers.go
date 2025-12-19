package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

// RandomString generates a random string of given length
func RandomString(length int) string {
	bytes := make([]byte, length/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// RandomUsername generates a random username
func RandomUsername() string {
	return fmt.Sprintf("user_%s", RandomString(8))
}

// RandomEmail generates a random email
func RandomEmail() string {
	return fmt.Sprintf("%s@test.com", RandomString(8))
}

// AssertEventually checks a condition repeatedly until timeout
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Condition not met within timeout: %s", message)
}

// WaitForCondition waits for a condition to be true
func WaitForCondition(condition func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}

	return false
}

// CleanupFunc is a function that cleans up resources
type CleanupFunc func()

// RegisterCleanup registers a cleanup function to run after test
func RegisterCleanup(t *testing.T, cleanup CleanupFunc) {
	t.Cleanup(cleanup)
}
