package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/service"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// --- request/response types ---

type createRequest struct {
	Recipient string          `json:"recipient"`
	Channel   string          `json:"channel"`
	Content   string          `json:"content"`
	Priority  string          `json:"priority"`
	Metadata  json.RawMessage `json:"metadata" swaggertype:"object"`
}

type notificationResponse struct {
	ID        string `json:"id"`
	BatchID   any    `json:"batch_id"`
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Content   string `json:"content"`
	Priority  string `json:"priority"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	MaxAttempts int  `json:"max_attempts"`
	Metadata  any    `json:"metadata"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type notificationWithAttemptsResponse struct {
	notificationResponse
	DeliveryAttempts []deliveryAttemptResponse `json:"delivery_attempts"`
}

type deliveryAttemptResponse struct {
	ID             string  `json:"id"`
	AttemptNumber  int     `json:"attempt_number"`
	Status         string  `json:"status"`
	HTTPStatusCode *int    `json:"http_status_code,omitempty"`
	LatencyMS      *int    `json:"latency_ms,omitempty"`
	AttemptedAt    string  `json:"attempted_at"`
}

type listResponse struct {
	Data       []notificationResponse `json:"data"`
	Pagination paginationMeta         `json:"pagination"`
}

type paginationMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type cancelResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type batchRequest struct {
	Notifications []createRequest `json:"notifications"`
}

type batchItemResult struct {
	Index  int        `json:"index"`
	Status string     `json:"status"`
	ID     *string    `json:"id,omitempty"`
	Error  *errorBody `json:"error,omitempty"`
}

type batchResponse struct {
	BatchID  string            `json:"batch_id"`
	Total    int               `json:"total"`
	Accepted int               `json:"accepted"`
	Rejected int               `json:"rejected"`
	Results  []batchItemResult `json:"results"`
}

// --- handler constructors ---

// createNotification godoc
// @Summary     Create a notification
// @Tags        notifications
// @Accept      json
// @Produce     json
// @Param       body body createRequest true "Notification payload"
// @Success     201 {object} notificationResponse
// @Failure     400 {object} errorBody
// @Failure     500 {object} errorBody
// @Router      /notifications [post]
func createNotification(svc service.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", nil)
			return
		}

		n, err := svc.Create(r.Context(), service.CreateRequest{
			Recipient: req.Recipient,
			Channel:   req.Channel,
			Content:   req.Content,
			Priority:  req.Priority,
			Metadata:  req.Metadata,
		})
		if err != nil {
			if service.IsValidationError(err) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			return
		}

		writeJSON(w, http.StatusCreated, toNotificationResponse(n))
	}
}

// getNotification godoc
// @Summary     Get a notification
// @Tags        notifications
// @Produce     json
// @Param       id path string true "Notification ID (UUID)"
// @Success     200 {object} notificationWithAttemptsResponse
// @Failure     400 {object} errorBody
// @Failure     404 {object} errorBody
// @Failure     500 {object} errorBody
// @Router      /notifications/{id} [get]
func getNotification(svc service.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid notification id", nil)
			return
		}

		n, attempts, err := svc.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "notification not found", nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			return
		}

		resp := notificationWithAttemptsResponse{
			notificationResponse: toNotificationResponse(n),
			DeliveryAttempts:     toAttemptResponses(attempts),
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// listNotifications godoc
// @Summary     List notifications
// @Tags        notifications
// @Produce     json
// @Param       status    query string false "Filter by status"
// @Param       channel   query string false "Filter by channel"
// @Param       batch_id  query string false "Filter by batch ID (UUID)"
// @Param       date_from query string false "Filter from date (RFC3339)"
// @Param       date_to   query string false "Filter to date (RFC3339)"
// @Param       sort      query string false "Sort field"
// @Param       order     query string false "Sort order (asc|desc)"
// @Param       page      query int    false "Page number (default 1)"
// @Param       page_size query int    false "Page size (default 20, max 100)"
// @Success     200 {object} listResponse
// @Failure     500 {object} errorBody
// @Router      /notifications [get]
func listNotifications(svc service.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		filter := db.ListFilter{
			Status:   q.Get("status"),
			Channel:  q.Get("channel"),
			Sort:     q.Get("sort"),
			Order:    q.Get("order"),
			Page:     intParam(q.Get("page"), 1),
			PageSize: intParam(q.Get("page_size"), 20),
		}
		if filter.PageSize > 100 {
			filter.PageSize = 100
		}

		if batchIDStr := q.Get("batch_id"); batchIDStr != "" {
			if id, err := uuid.Parse(batchIDStr); err == nil {
				filter.BatchID = &id
			}
		}
		if df := q.Get("date_from"); df != "" {
			if t, err := time.Parse(time.RFC3339, df); err == nil {
				filter.DateFrom = &t
			}
		}
		if dt := q.Get("date_to"); dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				filter.DateTo = &t
			}
		}

		ns, total, err := svc.List(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			return
		}

		data := make([]notificationResponse, len(ns))
		for i, n := range ns {
			data[i] = toNotificationResponse(n)
		}

		totalPages := (total + filter.PageSize - 1) / filter.PageSize
		if totalPages == 0 {
			totalPages = 1
		}

		writeJSON(w, http.StatusOK, listResponse{
			Data: data,
			Pagination: paginationMeta{
				Page:       filter.Page,
				PageSize:   filter.PageSize,
				Total:      total,
				TotalPages: totalPages,
			},
		})
	}
}

// cancelNotification godoc
// @Summary     Cancel a notification
// @Tags        notifications
// @Produce     json
// @Param       id path string true "Notification ID (UUID)"
// @Success     200 {object} cancelResponse
// @Failure     400 {object} errorBody
// @Failure     404 {object} errorBody
// @Failure     409 {object} errorBody
// @Failure     500 {object} errorBody
// @Router      /notifications/{id}/cancel [post]
func cancelNotification(svc service.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid notification id", nil)
			return
		}

		n, err := svc.Cancel(r.Context(), id)
		if err != nil {
			if errors.Is(err, db.ErrTransitionFailed) {
				writeError(w, http.StatusConflict, "INVALID_STATUS_TRANSITION", "notification cannot be cancelled in its current status", nil)
				return
			}
			if errors.Is(err, db.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "notification not found", nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			return
		}

		writeJSON(w, http.StatusOK, cancelResponse{
			ID:        n.ID.String(),
			Status:    n.Status,
			UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
		})
	}
}

// createBatch godoc
// @Summary     Create a batch of notifications
// @Tags        notifications
// @Accept      json
// @Produce     json
// @Param       body body batchRequest true "Batch payload (max 1000 items)"
// @Success     207 {object} batchResponse
// @Failure     400 {object} errorBody
// @Failure     500 {object} errorBody
// @Router      /notifications/batch [post]
func createBatch(svc service.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req batchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", nil)
			return
		}
		if len(req.Notifications) > 1000 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "batch size exceeds 1000", nil)
			return
		}

		svcReqs := make([]service.CreateRequest, len(req.Notifications))
		for i, item := range req.Notifications {
			svcReqs[i] = service.CreateRequest{
				Recipient: item.Recipient,
				Channel:   item.Channel,
				Content:   item.Content,
				Priority:  item.Priority,
				Metadata:  item.Metadata,
			}
		}

		batchID, results, err := svc.CreateBatch(r.Context(), svcReqs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			return
		}

		accepted := 0
		itemResults := make([]batchItemResult, 0, len(results))
		for _, res := range results {
			item := batchItemResult{Index: res.Index, Status: res.Status}
			if res.ID != nil {
				idStr := res.ID.String()
				item.ID = &idStr
				accepted++
			}
			if res.Error != nil {
				item.Error = &errorBody{Code: "VALIDATION_ERROR", Message: *res.Error}
			}
			itemResults = append(itemResults, item)
		}

		writeJSON(w, http.StatusMultiStatus, batchResponse{
			BatchID:  batchID.String(),
			Total:    len(req.Notifications),
			Accepted: accepted,
			Rejected: len(req.Notifications) - accepted,
			Results:  itemResults,
		})
	}
}

// --- mapping helpers ---

func toNotificationResponse(n *model.Notification) notificationResponse {
	var batchID any
	if n.BatchID != nil {
		batchID = n.BatchID.String()
	}
	return notificationResponse{
		ID:          n.ID.String(),
		BatchID:     batchID,
		Recipient:   n.Recipient,
		Channel:     n.Channel,
		Content:     n.Content,
		Priority:    n.Priority,
		Status:      n.Status,
		Attempts:    n.Attempts,
		MaxAttempts: n.MaxAttempts,
		Metadata:    n.Metadata,
		CreatedAt:   n.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   n.UpdatedAt.Format(time.RFC3339),
	}
}

func toAttemptResponses(attempts []*model.DeliveryAttempt) []deliveryAttemptResponse {
	if attempts == nil {
		return []deliveryAttemptResponse{}
	}
	out := make([]deliveryAttemptResponse, len(attempts))
	for i, a := range attempts {
		out[i] = deliveryAttemptResponse{
			ID:             a.ID.String(),
			AttemptNumber:  a.AttemptNumber,
			Status:         a.Status,
			HTTPStatusCode: a.HTTPStatusCode,
			LatencyMS:      a.LatencyMS,
			AttemptedAt:    a.AttemptedAt.Format(time.RFC3339),
		}
	}
	return out
}

func intParam(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}
