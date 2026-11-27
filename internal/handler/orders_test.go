package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"example.com/order-service/internal/order"
)

type stubService struct {
	err error
}

func (s stubService) Create(context.Context, order.CreateOrderInput) (order.Order, error) {
	return order.Order{}, s.err
}

func (s stubService) GetByID(context.Context, int64) (order.Order, error) {
	return order.Order{}, s.err
}

func (s stubService) List(context.Context) ([]order.Order, error) {
	return nil, s.err
}

func (s stubService) UpdateStatus(context.Context, int64, order.Status) (order.Order, error) {
	return order.Order{}, s.err
}

func (s stubService) History(context.Context, int64) ([]order.StatusChange, error) {
	return nil, s.err
}

func TestRouterStatusCodes(t *testing.T) {
	validOrder := `{"items":[{"name":"Keyboard","quantity":1,"price":500000}]}`
	wrapped := func(err error) error { return fmt.Errorf("service: %w", err) }

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		serviceErr error
		wantStatus int
		wantCode   string
		wantAllow  []string
	}{
		{name: "health", method: http.MethodGet, path: "/health", wantStatus: http.StatusOK},
		{name: "health head", method: http.MethodHead, path: "/health", wantStatus: http.StatusOK},
		{name: "create", method: http.MethodPost, path: "/api/v1/orders", body: validOrder, wantStatus: http.StatusCreated},
		{name: "list", method: http.MethodGet, path: "/api/v1/orders", wantStatus: http.StatusOK},
		{name: "get", method: http.MethodGet, path: "/api/v1/orders/1", wantStatus: http.StatusOK},
		{name: "update status", method: http.MethodPatch, path: "/api/v1/orders/1/status", body: `{"status":"processing"}`, wantStatus: http.StatusOK},
		{name: "history", method: http.MethodGet, path: "/api/v1/orders/1/history", wantStatus: http.StatusOK},
		{name: "broken json", method: http.MethodPost, path: "/api/v1/orders", body: `{"items":`, wantStatus: http.StatusBadRequest, wantCode: codeInvalidJSON},
		{name: "unknown field", method: http.MethodPost, path: "/api/v1/orders", body: `{"items":[],"x":1}`, wantStatus: http.StatusBadRequest, wantCode: codeInvalidJSON},
		{name: "two objects", method: http.MethodPost, path: "/api/v1/orders", body: validOrder + validOrder, wantStatus: http.StatusBadRequest, wantCode: codeInvalidJSON},
		{name: "body too large", method: http.MethodPost, path: "/api/v1/orders", body: `{"items":"` + strings.Repeat("a", maxBodyBytes) + `"}`, wantStatus: http.StatusRequestEntityTooLarge, wantCode: codeBodyTooLarge},
		{name: "validation error", method: http.MethodPost, path: "/api/v1/orders", body: validOrder, serviceErr: wrapped(&order.ValidationError{Field: "items", Reason: "must contain at least one item"}), wantStatus: http.StatusBadRequest, wantCode: codeValidationError},
		{name: "id not a number", method: http.MethodGet, path: "/api/v1/orders/abc", wantStatus: http.StatusBadRequest, wantCode: codeInvalidID},
		{name: "id with plus sign", method: http.MethodGet, path: "/api/v1/orders/+1", wantStatus: http.StatusBadRequest, wantCode: codeInvalidID},
		{name: "id with leading zeros", method: http.MethodGet, path: "/api/v1/orders/0001", wantStatus: http.StatusBadRequest, wantCode: codeInvalidID},
		{name: "too large after first object", method: http.MethodPost, path: "/api/v1/orders", body: validOrder + strings.Repeat(" ", maxBodyBytes), wantStatus: http.StatusRequestEntityTooLarge, wantCode: codeBodyTooLarge},
		{name: "id zero", method: http.MethodGet, path: "/api/v1/orders/0/history", wantStatus: http.StatusBadRequest, wantCode: codeInvalidID},
		{name: "invalid status", method: http.MethodPatch, path: "/api/v1/orders/1/status", body: `{"status":"shipped"}`, serviceErr: wrapped(order.ErrInvalidStatus), wantStatus: http.StatusBadRequest, wantCode: codeInvalidStatus},
		{name: "order not found", method: http.MethodGet, path: "/api/v1/orders/42", serviceErr: wrapped(order.ErrOrderNotFound), wantStatus: http.StatusNotFound, wantCode: codeOrderNotFound},
		{name: "transition not allowed", method: http.MethodPatch, path: "/api/v1/orders/1/status", body: `{"status":"new"}`, serviceErr: wrapped(order.ErrInvalidStatusTransition), wantStatus: http.StatusConflict, wantCode: codeInvalidStatusTransition},
		{name: "unexpected error", method: http.MethodGet, path: "/api/v1/orders", serviceErr: errors.New("db password leaked"), wantStatus: http.StatusInternalServerError, wantCode: codeInternalError},
		{name: "unknown route", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound, wantCode: codeNotFound},
		{name: "wrong method", method: http.MethodDelete, path: "/api/v1/orders/1", wantStatus: http.StatusMethodNotAllowed, wantCode: codeMethodNotAllowed, wantAllow: []string{http.MethodGet, http.MethodHead}},
		{name: "wrong method on collection", method: http.MethodPut, path: "/api/v1/orders", wantStatus: http.StatusMethodNotAllowed, wantCode: codeMethodNotAllowed, wantAllow: []string{http.MethodGet, http.MethodHead, http.MethodPost}},
		{name: "wrong method on encoded path", method: http.MethodPut, path: "/api/v1/orders/a%2Fb", wantStatus: http.StatusMethodNotAllowed, wantCode: codeMethodNotAllowed, wantAllow: []string{http.MethodGet, http.MethodHead}},
		{name: "null body on create", method: http.MethodPost, path: "/api/v1/orders", body: "null", wantStatus: http.StatusBadRequest, wantCode: codeInvalidJSON},
		{name: "null body on status", method: http.MethodPatch, path: "/api/v1/orders/1/status", body: " null ", wantStatus: http.StatusBadRequest, wantCode: codeInvalidJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(stubService{err: tt.serviceErr}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.NotEmpty(t, rec.Header().Get(headerRequestID))
			assert.Equal(t, tt.wantAllow, rec.Header().Values("Allow"))
			if tt.wantCode == "" {
				return
			}
			var body errorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.wantCode, body.Code)
			assert.NotEmpty(t, body.Error)
			assert.NotContains(t, body.Error, "password")
		})
	}
}
