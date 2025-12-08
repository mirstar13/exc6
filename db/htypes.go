package db

import "sync"

type User struct {
	UserId     string `json:"user_id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	Password   string `json:"password"`
	Icon       string `json:"icon"`
	CustomIcon string `json:"custom_icon"` // Path to uploaded image
}

type UsersDB struct {
	Users []*User `json:"users"`
	mu    sync.RWMutex
	path  string
}
