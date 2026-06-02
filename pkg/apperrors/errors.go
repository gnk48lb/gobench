package apperrors

import "errors"

var (
	ErrUsernameExists     = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrTaskNotFound       = errors.New("task not found")
)
