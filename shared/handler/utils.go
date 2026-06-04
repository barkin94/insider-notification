package handler

import (
	"encoding/json"
	"net/http"
)

// AppHandler is an http.HandlerFunc that can return an error.
type AppHandler func(w http.ResponseWriter, r *http.Request) error

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}

// DecodeBody decodes the JSON request body into T.
func DecodeBody[T any](r *http.Request) (T, error) {
	var body T
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return body, err
	}
	return body, nil
}
