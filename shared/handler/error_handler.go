package handler

import (
	"errors"
	"log/slog"
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

// ErrorEntry maps a sentinel error to an HTTP response.
type ErrorEntry struct {
	Status  int
	Code    string
	Message string // empty: use err.Error()
}

// ErrorMap maps sentinel errors (matched via errors.Is) to HTTP responses.
// It is checked after AppError and before the generic 500 fallback.
type ErrorMap map[error]ErrorEntry

// ErrorHandler adapts an AppHandler into a standard http.HandlerFunc.
// AppError values are written as JSON; any other error becomes a 500.
func ErrorHandler(h AppHandler) http.HandlerFunc {
	return errHandler(h, nil)
}

func errHandler(h AppHandler, m ErrorMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			slog.ErrorContext(r.Context(), "error", "error", err.Error())

			var appErr *AppError
			if errors.As(err, &appErr) {
				writeError(w, appErr.Status, appErr.Code, appErr.Message, nil)
				return
			}

			for sentinel, entry := range m {
				if errors.Is(err, sentinel) {
					msg := entry.Message
					if msg == "" {
						msg = err.Error()
					}
					writeError(w, entry.Status, entry.Code, msg, nil)
					return
				}
			}

			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		}
	}
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message, Details: details}})
}
