package handler

import (
	"net/http"

	"github.com/barkin/insider-notification/api/internal/middleware"
)

// errorBody is used for per-item errors in batch responses.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func errBadRequest(code, msg string) *middleware.AppError {
	return &middleware.AppError{Status: http.StatusBadRequest, Code: code, Message: msg}
}

func errNotFound(msg string) *middleware.AppError {
	return &middleware.AppError{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: msg}
}

func errConflict(code, msg string) *middleware.AppError {
	return &middleware.AppError{Status: http.StatusConflict, Code: code, Message: msg}
}

func errInternal() *middleware.AppError {
	return &middleware.AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "internal server error"}
}
