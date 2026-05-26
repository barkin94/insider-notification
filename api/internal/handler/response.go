package handler

import "net/http"

func errBadRequest(code, msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: code, Message: msg}
}

func errNotFound(msg string) *AppError {
	return &AppError{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: msg}
}

func errConflict(code, msg string) *AppError {
	return &AppError{Status: http.StatusConflict, Code: code, Message: msg}
}

func errInternal() *AppError {
	return &AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "internal server error"}
}
