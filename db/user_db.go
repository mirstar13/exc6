package db

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 8
	MaxPasswordLength = 128
	BCryptCost        = bcrypt.DefaultCost
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrWeakPassword      = errors.New("password too weak")
)

func OpenUsersDB() (*UsersDB, error) {
	return OpenUsersDBFromPath("users.json")
}

func OpenUsersDBFromPath(path string) (*UsersDB, error) {
	db := &UsersDB{
		Users: make([]*User, 0),
		path:  path,
	}

	// Try to read existing file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, start with empty DB
			return db, nil
		}
		return nil, fmt.Errorf("failed to read database: %w", err)
	}

	if err := json.Unmarshal(data, db); err != nil {
		return nil, fmt.Errorf("failed to parse database: %w", err)
	}

	return db, nil
}

func (udb *UsersDB) Save() error {
	udb.mu.RLock()
	defer udb.mu.RUnlock()

	data, err := json.MarshalIndent(udb, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal database: %w", err)
	}

	// Write to temp file first, then rename (atomic operation)
	tempPath := udb.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, udb.path); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to save database: %w", err)
	}

	return nil
}

func (udb *UsersDB) AddUser(user User) error {
	udb.mu.Lock()
	defer udb.mu.Unlock()

	// Check if user already exists
	for _, u := range udb.Users {
		if u.Username == user.Username {
			return ErrUserAlreadyExists
		}
	}

	// Generate user ID
	userID, err := generateUserID()
	if err != nil {
		return fmt.Errorf("failed to generate user ID: %w", err)
	}
	user.UserId = userID

	udb.Users = append(udb.Users, &user)
	return nil
}

func (udb *UsersDB) ValidateCredentials(username, password string) bool {
	udb.mu.RLock()
	defer udb.mu.RUnlock()

	for _, user := range udb.Users {
		if user.Username == username {
			err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
			return err == nil
		}
	}
	return false
}

func (udb *UsersDB) FindUserByUsername(username string) *User {
	udb.mu.RLock()
	defer udb.mu.RUnlock()

	for _, user := range udb.Users {
		if user.Username == username {
			// Return a copy to prevent external modifications
			userCopy := *user
			return &userCopy
		}
	}
	return nil
}

func (udb *UsersDB) FindUserById(userID string) *User {
	udb.mu.RLock()
	defer udb.mu.RUnlock()

	for _, user := range udb.Users {
		if user.UserId == userID {
			// Return a copy to prevent external modifications
			userCopy := *user
			return &userCopy
		}
	}
	return nil
}

func (udb *UsersDB) UpdateUser(username string, updateFn func(*User) error) error {
	udb.mu.Lock()
	defer udb.mu.Unlock()

	for _, user := range udb.Users {
		if user.Username == username {
			return updateFn(user)
		}
	}
	return ErrUserNotFound
}

func (udb *UsersDB) GetAllUsernames() []string {
	udb.mu.RLock()
	defer udb.mu.RUnlock()

	usernames := make([]string, 0, len(udb.Users))
	for _, user := range udb.Users {
		usernames = append(usernames, user.Username)
	}
	return usernames
}

// ValidatePasswordStrength checks if password meets minimum requirements
func ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("%w: must be at least %d characters", ErrWeakPassword, MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("%w: must be less than %d characters", ErrWeakPassword, MaxPasswordLength)
	}
	// Add more rules as needed (uppercase, numbers, special chars, etc.)
	return nil
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BCryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

func generateUserID() (string, error) {
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", randBytes), nil
}
