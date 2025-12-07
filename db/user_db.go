package db

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// TODO: use goose with postgres instead of a json file, jesus

func OpenUsersDB() (*UsersDB, error) {
	dbFile, err := os.OpenFile("users.json", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer dbFile.Close()

	var db UsersDB
	decoder := json.NewDecoder(dbFile)
	decoder.Decode(&db)

	return &db, nil
}

func (udb *UsersDB) Save() error {
	dbFile, err := os.OpenFile("users.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer dbFile.Close()

	data, err := json.MarshalIndent(udb, "", "  ")
	if err != nil {
		return err
	}

	if _, err = dbFile.Write(data); err != nil {
		return err
	}

	return nil
}

func (udb *UsersDB) AddUser(user User) {
	user.UserId, _ = generateUserID()
	udb.Users = append(udb.Users, &user)
}

func (udb *UsersDB) ValidateCredentials(username, password string) bool {
	for _, user := range udb.Users {
		if user.Username == username {
			err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))

			if err != nil {
				return false
			} else {
				return true
			}
		}
	}
	return false
}

func (udb *UsersDB) FindUserByUsername(username string) *User {
	for _, user := range udb.Users {
		if user.Username == username {
			return user
		}
	}
	return nil
}

func (udb *UsersDB) FindUserById(userID string) *User {
	for _, user := range udb.Users {
		if user.UserId == userID {
			return user
		}
	}
	return nil
}

func (udb *UsersDB) GetAllUsernames() []string {
	usernames := make([]string, 0, len(udb.Users))
	for _, user := range udb.Users {
		usernames = append(usernames, user.Username)
	}
	return usernames
}

func generateUserID() (string, error) {
	randId := make([]byte, 16)
	if _, err := rand.Read(randId); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", randId), nil
}
