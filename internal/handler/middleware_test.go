package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryMiddlewareReturnsSafeJSON(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	panicking := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		panic("secret internal detail")
	})
	h := requestIDMiddleware(requestLoggingMiddleware(logger)(recoveryMiddleware(logger)(panicking)))

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get(headerRequestID))
	var body errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, errorResponse{Error: "internal server error", Code: codeInternalError}, body)
	assert.NotContains(t, rec.Body.String(), "secret")
	assert.Contains(t, logs.String(), `"msg":"panic recovered"`)
	assert.Contains(t, logs.String(), `"status":500`)
	assert.Contains(t, logs.String(), `"request_id":"`)
}

func TestRecoveryMiddlewareAbortsStartedResponse(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	panicking := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
		panic("late failure")
	})
	h := requestIDMiddleware(requestLoggingMiddleware(logger)(recoveryMiddleware(logger)(panicking)))

	rec := httptest.NewRecorder()
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	})

	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
	assert.Contains(t, logs.String(), `"response_started":true`)
	assert.Contains(t, logs.String(), `"aborted":true`)
}
