package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

const headerRequestID = "X-Request-ID"

type requestIDKey struct{}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw [8]byte
		_, _ = rand.Read(raw[:])
		id := hex.EncodeToString(raw[:])
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func wrapWriter(w http.ResponseWriter, r *http.Request) middleware.WrapResponseWriter {
	if ww, ok := w.(middleware.WrapResponseWriter); ok {
		return ww
	}
	return middleware.NewWrapResponseWriter(w, r.ProtoMajor)
}

func requestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := wrapWriter(w, r)
			defer func() {
				aborted := recover()
				status := ww.Status()
				if status == 0 && aborted == nil {
					status = http.StatusOK
				}
				logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", status),
					slog.Bool("aborted", aborted != nil),
					slog.String("duration", time.Since(start).String()),
					slog.String("request_id", requestID(r)),
				)
				if aborted != nil {
					panic(aborted)
				}
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := wrapWriter(w, r)
			defer func() {
				value := recover()
				if value == nil {
					return
				}
				if value == http.ErrAbortHandler {
					panic(value)
				}
				started := ww.Status() != 0
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.String("request_id", requestID(r)),
					slog.String("panic", fmt.Sprint(value)),
					slog.Bool("response_started", started),
					slog.String("stack", string(debug.Stack())),
				)
				if started {
					panic(http.ErrAbortHandler)
				}
				clear(ww.Header())
				ww.Header().Set(headerRequestID, requestID(r))
				writeError(ww, http.StatusInternalServerError, codeInternalError, "internal server error")
			}()
			next.ServeHTTP(ww, r)
		})
	}
}
