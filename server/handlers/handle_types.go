package handlers

// Will be used for JSON-based API endpoints in the future
type RequestUserRegister struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Will be used for JSON-based API endpoints in the future
type RequestUserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Will be used for JSON-based API endpoints in the future
type ResponseUserRegister struct {
	UserId string `json:"user_id"`
}

// Will be used for JSON-based API endpoints in the future
type ResponseUserLogin struct {
	SessionToken string `json:"session_token"`
}
