package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "example.com/order-service/docs"
	"example.com/order-service/internal/handler"
	"example.com/order-service/internal/repository/memory"
	"example.com/order-service/internal/service"
)

// @title        Order Service API
// @version      1.0
// @description  Backend service for orders: create orders, read one order or the list, change status and read status history. Data is kept in memory. Prices are integers in minor currency units, for example kopecks.
// @BasePath     /
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("HTTP server stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("HTTP server stopped")
}

func run(logger *slog.Logger) error {
	repository := memory.NewOrderRepository()
	orderService := service.NewOrderService(repository, time.Now)
	router := handler.NewRouter(orderService, logger)

	address := cmp.Or(os.Getenv("HTTP_ADDR"), ":8080")
	server := &http.Server{
		Addr:              address,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("starting HTTP server", slog.String("address", address))
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("listen on %q: %w", address, err)
	case <-ctx.Done():
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
