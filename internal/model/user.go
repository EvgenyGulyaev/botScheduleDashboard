package model

type UserData struct {
	Login          string `json:"login"`
	Email          string `json:"email"`
	HashedPassword []byte `json:"hashed_password"`
	IsAdmin        bool   `json:"is_admin"`
}
