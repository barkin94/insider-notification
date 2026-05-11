package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/middleware"
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
	PageSize   int     `json:"page_size"`
	Total      int     `json:"total"`
	NextCursor *string `json:"next_cursor"`
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
func createNotification(svc service.NotificationService) middleware.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req createRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return errBadRequest("VALIDATION_ERROR", "invalid JSON body")
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
				return errBadRequest("VALIDATION_ERROR", err.Error())
			}
			return errInternal()
		}

		middleware.WriteJSON(w, http.StatusCreated, toNotificationResponse(n))
		return nil
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
func getNotification(svc service.NotificationService) middleware.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			return errBadRequest("VALIDATION_ERROR", "invalid notification id")
		}

		n, attempts, err := svc.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return errNotFound("notification not found")
			}
			return errInternal()
		}

		middleware.WriteJSON(w, http.StatusOK, notificationWithAttemptsResponse{
			notificationResponse: toNotificationResponse(n),
			DeliveryAttempts:     toAttemptResponses(attempts),
		})
		return nil
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
// @Param       page_size query int    false "Page size (default 20, max 100)"
// @Param       cursor    query string false "Opaque cursor for keyset pagination (base64url-encoded UUID)"
// @Success     200 {object} listResponse
// @Failure     400 {object} errorBody
// @Failure     500 {object} errorBody
// @Router      /notifications [get]
func listNotifications(svc service.NotificationService) middleware.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		q := r.URL.Query()

		pageSize := intParam(q.Get("page_size"), 20)
		if pageSize > 100 {
			pageSize = 100
		}

		var batchID *uuid.UUID
		if s := q.Get("batch_id"); s != "" {
			if id, err := uuid.Parse(s); err == nil {
				batchID = &id
			}
		}
		var dateFrom, dateTo *time.Time
		if s := q.Get("date_from"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				dateFrom = &t
			}
		}
		if s := q.Get("date_to"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				dateTo = &t
			}
		}

		f := db.ListFilter{
			Status:   q.Get("status"),
			Channel:  q.Get("channel"),
			BatchID:  batchID,
			DateFrom: dateFrom,
			DateTo:   dateTo,
			Sort:     q.Get("sort"),
			Order:    q.Get("order"),
			Page:     intParam(q.Get("page"), 1),
			PageSize: pageSize,
		}

		if cursorStr := q.Get("cursor"); cursorStr != "" {
			cursorID, err := decodeCursor(cursorStr)
			if err != nil {
				return errBadRequest("VALIDATION_ERROR", "invalid cursor")
			}
			f.CursorID = cursorID
		}

		ns, total, nextCursor, err := svc.List(r.Context(), f)
		if err != nil {
			return errInternal()
		}
		middleware.WriteJSON(w, http.StatusOK, listResponse{
			Data: toNotificationResponses(ns),
			Pagination: paginationMeta{
				PageSize:   pageSize,
				Total:      total,
				NextCursor: encodeCursor(nextCursor),
			},
		})
		return nil
	}
}

func decodeCursor(s string) (*uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	id, err := uuid.ParseBytes(b)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func encodeCursor(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := base64.RawURLEncoding.EncodeToString([]byte(id.String()))
	return &s
}

func toNotificationResponses(ns []*model.Notification) []notificationResponse {
	data := make([]notificationResponse, len(ns))
	for i, n := range ns {
		data[i] = toNotificationResponse(n)
	}
	return data
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
func cancelNotification(svc service.NotificationService) middleware.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			return errBadRequest("VALIDATION_ERROR", "invalid notification id")
		}

		n, err := svc.Cancel(r.Context(), id)
		if err != nil {
			if errors.Is(err, db.ErrTransitionFailed) {
				return errConflict("INVALID_STATUS_TRANSITION", "notification cannot be cancelled in its current status")
			}
			if errors.Is(err, db.ErrNotFound) {
				return errNotFound("notification not found")
			}
			return errInternal()
		}

		middleware.WriteJSON(w, http.StatusOK, cancelResponse{
			ID:        n.ID.String(),
			Status:    n.Status,
			UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
		})
		return nil
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
func createBatch(svc service.NotificationService) middleware.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req batchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return errBadRequest("VALIDATION_ERROR", "invalid JSON body")
		}
		if len(req.Notifications) > 1000 {
			return errBadRequest("VALIDATION_ERROR", "batch size exceeds 1000")
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
			return errInternal()
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

		middleware.WriteJSON(w, http.StatusMultiStatus, batchResponse{
			BatchID:  batchID.String(),
			Total:    len(req.Notifications),
			Accepted: accepted,
			Rejected: len(req.Notifications) - accepted,
			Results:  itemResults,
		})
		return nil
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
