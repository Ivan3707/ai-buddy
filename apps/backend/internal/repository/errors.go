package repository

import "errors"

var (
	ErrNotFound           = errors.New("repository: record not found")
	ErrEmailAlreadyExists = errors.New("repository: email already exists")
	ErrConflict           = errors.New("repository: conflict")
)
