package handler

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"example.com/order-service/internal/order"
)

type OrderService interface {
	Create(ctx context.Context, input order.CreateOrderInput) (order.Order, error)
	GetByID(ctx context.Context, id int64) (order.Order, error)
	List(ctx context.Context) ([]order.Order, error)
	UpdateStatus(ctx context.Context, id int64, status order.Status) (order.Order, error)
	History(ctx context.Context, id int64) ([]order.StatusChange, error)
}

type api struct {
	service OrderService
	logger  *slog.Logger
}

func NewRouter(service OrderService, logger *slog.Logger) http.Handler {
	h := &api{service: service, logger: logger}
	router := chi.NewRouter()
	router.Use(requestIDMiddleware)
	router.Use(requestLoggingMiddleware(logger))
	router.Use(recoveryMiddleware(logger))
	router.Use(middleware.GetHead)

	router.Get("/health", h.health)
	router.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	router.Post("/api/v1/orders", h.createOrder)
	router.Get("/api/v1/orders", h.listOrders)
	router.Get("/api/v1/orders/{id}", h.getOrder)
	router.Patch("/api/v1/orders/{id}/status", h.updateStatus)
	router.Get("/api/v1/orders/{id}/history", h.orderHistory)

	methods := registeredMethods(router)
	router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, codeNotFound, "resource not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		for _, method := range allowedMethods(router, methods, r) {
			w.Header().Add("Allow", method)
		}
		writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "method not allowed")
	})

	return router
}

func registeredMethods(router chi.Routes) []string {
	var methods []string
	_ = chi.Walk(router, func(method, _ string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !slices.Contains(methods, method) {
			methods = append(methods, method)
		}
		return nil
	})
	slices.Sort(methods)
	return methods
}

func allowedMethods(router chi.Routes, methods []string, r *http.Request) []string {
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	var allowed []string
	for _, method := range methods {
		if !router.Match(chi.NewRouteContext(), method, path) {
			continue
		}
		allowed = append(allowed, method)
		if method == http.MethodGet {
			allowed = append(allowed, http.MethodHead)
		}
	}
	return allowed
}
