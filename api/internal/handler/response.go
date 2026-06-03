package handler

import (
	"net/http"

	sharedhandler "github.com/barkin/insider-notification/shared/handler"
)

func errBadRequest(code, msg string) *sharedhandler.AppError {
	return &sharedhandler.AppError{Status: http.StatusBadRequest, Code: code, Message: msg}
}

func errNotFound(msg string) *sharedhandler.AppError {
	return &sharedhandler.AppError{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: msg}
}

func errConflict(code, msg string) *sharedhandler.AppError {
	return &sharedhandler.AppError{Status: http.StatusConflict, Code: code, Message: msg}
}

func errInternal() *sharedhandler.AppError {
	return &sharedhandler.AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "internal server error"}
}
