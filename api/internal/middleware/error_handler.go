package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

// AppError is an HTTP-aware error returned by AppHandler functions.
type AppError struct {
	Status  int
	Code    string
	Message string
}

func (e *AppError) Error() string { return e.Message }

// AppHandler is an http.HandlerFunc that can return an error.
type AppHandler func(w http.ResponseWriter, r *http.Request) error

// ErrorHandler adapts an AppHandler into a standard http.HandlerFunc.
// AppError values are written as JSON; any other error becomes a 500.
func ErrorHandler(h AppHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			var appErr *AppError
			if errors.As(err, &appErr) {
				writeError(w, appErr.Status, appErr.Code, appErr.Message, nil)
			} else {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			}
		}
	}
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message, Details: details}})
}
