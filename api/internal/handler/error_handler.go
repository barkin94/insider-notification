package handler

// errorBody is the per-item error shape used in batch responses.
// The top-level HTTP error envelope is owned by shared/handler.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
