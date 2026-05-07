package db

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrTransitionFailed = errors.New("status transition failed: unexpected current status")
)
