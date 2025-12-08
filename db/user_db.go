package db

import (
	"crypto/rand"
	"encoding/json"
	"exc6/apperrors"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 8
	MaxPasswordLength = 128
	BCryptCost        = bcrypt.DefaultCost
)

func OpenUsersDB() (*UsersDB, error) {
	return OpenUsersDBFromPath("users.json")
}

func OpenUsersDBFromPath(path string) (*UsersDB, error) {
	db := &UsersDB{
		Users: make([]*User, 0),
		path:  path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return db, nil
		}
		return nil, apperrors.NewDatabaseError("read", err)
	}

	if err := json.Unmarshal(data, db); err != nil {
		return nil, apperrors.NewDatabaseError("parse", err)
	}

	return db, nil
}

func (udb *UsersDB) Save() error {
	udb.mu.RLock()
	defer udb.mu.RUnlock()

	data, err := json.MarshalIndent(udb, "", "  ")
	if err != nil {
		return apperrors.NewDatabaseError("marshal", err)
	}

	tempPath := udb.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return apperrors.NewDatabaseError("write temp file", err)
	}

	if err := os.Rename(tempPath, udb.path); err != nil {
		os.Remove(tempPath)
		return apperrors.NewDatabaseError("save", err)
	}

	return nil
}

func (udb *UsersDB) AddUser(user User) error {
	udb.mu.Lock()
	defer udb.mu.Unlock()

	// Check if user already exists
	for _, u := range udb.Users {
		if u.Username == user.Username {
			return apperrors.NewUserExists(user.Username)
		}
	}

	// Generate user ID
	userID, err := generateUserID()
	if err != nil {
		return apperrors.NewInternalError("failed to generate user ID").WithInternal(err)
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
	return apperrors.NewUserNotFound()
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

// ValidatePasswordStrength checks if password meets requirements
func ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return apperrors.NewWeakPassword(fmt.Sprintf("must be at least %d characters", MinPasswordLength))
	}
	if len(password) > MaxPasswordLength {
		return apperrors.NewWeakPassword(fmt.Sprintf("must be less than %d characters", MaxPasswordLength))
	}
	return nil
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BCryptCost)
	if err != nil {
		return "", apperrors.NewInternalError("failed to hash password").WithInternal(err)
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
