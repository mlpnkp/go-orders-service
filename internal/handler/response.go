package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"example.com/order-service/internal/order"
)

const (
	codeInvalidJSON             = "INVALID_JSON"
	codeBodyTooLarge            = "BODY_TOO_LARGE"
	codeInvalidID               = "INVALID_ID"
	codeValidationError         = "VALIDATION_ERROR"
	codeInvalidStatus           = "INVALID_STATUS"
	codeOrderNotFound           = "ORDER_NOT_FOUND"
	codeInvalidStatusTransition = "INVALID_STATUS_TRANSITION"
	codeInternalError           = "INTERNAL_ERROR"
	codeNotFound                = "NOT_FOUND"
	codeMethodNotAllowed        = "METHOD_NOT_ALLOWED"
)

type errorResponse struct {
	Error string `json:"error" example:"status transition is not allowed"`
	Code  string `json:"code" example:"INVALID_STATUS_TRANSITION"`
}

type healthResponse struct {
	Status string `json:"status" example:"ok"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: message, Code: code})
}

func (h *api) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *order.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeError(w, http.StatusBadRequest, codeValidationError, validationErr.Error())
	case errors.Is(err, order.ErrInvalidStatus):
		writeError(w, http.StatusBadRequest, codeInvalidStatus,
			"status must be one of: new, processing, completed, cancelled")
	case errors.Is(err, order.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, codeOrderNotFound, "order not found")
	case errors.Is(err, order.ErrInvalidStatusTransition):
		writeError(w, http.StatusConflict, codeInvalidStatusTransition, "status transition is not allowed")
	default:
		h.logger.ErrorContext(r.Context(), "request failed",
			slog.String("request_id", requestID(r)), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal server error")
	}
}
