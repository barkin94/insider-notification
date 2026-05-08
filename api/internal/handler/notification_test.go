package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/handler"
	"github.com/barkin/insider-notification/api/internal/service"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/google/uuid"
)

// --- mock service ---

type mockService struct {
	createFn      func(ctx context.Context, req service.CreateRequest) (*model.Notification, error)
	getByIDFn     func(ctx context.Context, id uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error)
	listFn        func(ctx context.Context, f db.ListFilter) ([]*model.Notification, int, error)
	cancelFn      func(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	createBatchFn func(ctx context.Context, reqs []service.CreateRequest) (uuid.UUID, []service.BatchResult, error)
}

func (m *mockService) Create(ctx context.Context, req service.CreateRequest) (*model.Notification, error) {
	return m.createFn(ctx, req)
}
func (m *mockService) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockService) List(ctx context.Context, f db.ListFilter) ([]*model.Notification, int, error) {
	return m.listFn(ctx, f)
}
func (m *mockService) Cancel(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	return m.cancelFn(ctx, id)
}
func (m *mockService) CreateBatch(ctx context.Context, reqs []service.CreateRequest) (uuid.UUID, []service.BatchResult, error) {
	return m.createBatchFn(ctx, reqs)
}

func newRouter(svc service.NotificationService) http.Handler {
	return handler.NewRouter(handler.Deps{
		Service: svc,
		DB:      nil, // health check not tested here
		Redis:   nil,
	})
}

func newNotif() *model.Notification {
	now := time.Now().UTC()
	return &model.Notification{
		ID:          uuid.New(),
		Recipient:   "+1",
		Channel:     model.ChannelSMS,
		Content:     "hi",
		Priority:    model.PriorityNormal,
		Status:      model.StatusPending,
		MaxAttempts: 4,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// --- POST /notifications ---

func TestCreateNotification_201(t *testing.T) {
	n := newNotif()
	svc := &mockService{createFn: func(_ context.Context, _ service.CreateRequest) (*model.Notification, error) {
		return n, nil
	}}

	body := `{"recipient":"+1","channel":"sms","content":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["id"] == nil {
		t.Error("response missing id field")
	}
	if resp["status"] != "pending" {
		t.Errorf("status = %v, want pending", resp["status"])
	}
}

func TestCreateNotification_400_missingContent(t *testing.T) {
	svc := &mockService{createFn: func(_ context.Context, req service.CreateRequest) (*model.Notification, error) {
		return nil, &service.ValidationError{Field: "content", Message: "required"}
	}}

	body := `{"recipient":"+1","channel":"sms"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("error code = %v, want VALIDATION_ERROR", errObj["code"])
	}
}

func TestCreateNotification_400_contentTooLong(t *testing.T) {
	svc := &mockService{createFn: func(_ context.Context, req service.CreateRequest) (*model.Notification, error) {
		return nil, &service.ValidationError{Field: "content", Message: "exceeds 1600 char limit for sms"}
	}}

	body := `{"recipient":"+1","channel":"sms","content":"` + strings.Repeat("x", 1601) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// --- GET /notifications ---

func TestListNotifications_pagination(t *testing.T) {
	svc := &mockService{listFn: func(_ context.Context, f db.ListFilter) ([]*model.Notification, int, error) {
		return []*model.Notification{newNotif()}, 42, nil
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?page=2&page_size=10", nil)
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	pg, _ := resp["pagination"].(map[string]any)
	if pg["page"] != float64(2) {
		t.Errorf("page = %v, want 2", pg["page"])
	}
	if pg["total"] != float64(42) {
		t.Errorf("total = %v, want 42", pg["total"])
	}
	if pg["total_pages"] != float64(5) {
		t.Errorf("total_pages = %v, want 5", pg["total_pages"])
	}
}

func TestListNotifications_filterByStatus(t *testing.T) {
	var gotFilter db.ListFilter
	svc := &mockService{listFn: func(_ context.Context, f db.ListFilter) ([]*model.Notification, int, error) {
		gotFilter = f
		return nil, 0, nil
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?status=delivered", nil)
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if gotFilter.Status != "delivered" {
		t.Errorf("filter status = %q, want delivered", gotFilter.Status)
	}
}

// --- GET /notifications/:id ---

func TestGetNotification_200(t *testing.T) {
	n := newNotif()
	latency := 100
	attempts := []*model.DeliveryAttempt{{
		ID:            uuid.New(),
		AttemptNumber: 1,
		Status:        "success",
		LatencyMS:     &latency,
		AttemptedAt:   time.Now().UTC(),
	}}
	svc := &mockService{getByIDFn: func(_ context.Context, _ uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error) {
		return n, attempts, nil
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/"+n.ID.String(), nil)
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	da, _ := resp["delivery_attempts"].([]any)
	if len(da) != 1 {
		t.Errorf("delivery_attempts len = %d, want 1", len(da))
	}
}

func TestGetNotification_404(t *testing.T) {
	svc := &mockService{getByIDFn: func(_ context.Context, _ uuid.UUID) (*model.Notification, []*model.DeliveryAttempt, error) {
		return nil, nil, db.ErrNotFound
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// --- POST /notifications/:id/cancel ---

func TestCancelNotification_200(t *testing.T) {
	n := newNotif()
	n.Status = model.StatusCancelled
	svc := &mockService{cancelFn: func(_ context.Context, _ uuid.UUID) (*model.Notification, error) {
		return n, nil
	}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+n.ID.String()+"/cancel", nil)
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["status"] != "cancelled" {
		t.Errorf("status = %v, want cancelled", resp["status"])
	}
}

func TestCancelNotification_409(t *testing.T) {
	svc := &mockService{cancelFn: func(_ context.Context, _ uuid.UUID) (*model.Notification, error) {
		return nil, db.ErrTransitionFailed
	}}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+uuid.New().String()+"/cancel", nil)
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != "INVALID_STATUS_TRANSITION" {
		t.Errorf("code = %v, want INVALID_STATUS_TRANSITION", errObj["code"])
	}
}

// --- POST /notifications/batch ---

func TestCreateBatch_207(t *testing.T) {
	id1 := uuid.New()
	svc := &mockService{createBatchFn: func(_ context.Context, reqs []service.CreateRequest) (uuid.UUID, []service.BatchResult, error) {
		errMsg := "validation: channel: must be one of: sms, email, push"
		return uuid.New(), []service.BatchResult{
			{Index: 0, Status: "accepted", ID: &id1},
			{Index: 1, Status: "rejected", Error: &errMsg},
		}, nil
	}}

	body, _ := json.Marshal(map[string]any{
		"notifications": []map[string]any{
			{"recipient": "+1", "channel": "sms", "content": "ok"},
			{"recipient": "+2", "channel": "fax", "content": "bad"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["accepted"] != float64(1) {
		t.Errorf("accepted = %v, want 1", resp["accepted"])
	}
	if resp["rejected"] != float64(1) {
		t.Errorf("rejected = %v, want 1", resp["rejected"])
	}
}

func TestCreateBatch_400_tooLarge(t *testing.T) {
	svc := &mockService{}

	items := make([]map[string]any, 1001)
	for i := range items {
		items[i] = map[string]any{"recipient": "+1", "channel": "sms", "content": "hi"}
	}
	body, _ := json.Marshal(map[string]any{"notifications": items})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
