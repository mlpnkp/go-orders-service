package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"example.com/order-service/internal/order"
)

const maxBodyBytes = 1 << 20

// @Summary      Health check
// @Description  Returns ok when the service is up.
// @Tags         health
// @Produce      json
// @Success      200  {object}  healthResponse
// @Router       /health [get]
func (h *api) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// @Summary      Create order
// @Description  Creates an order with status new. Every item needs a non-empty name, quantity > 0 and price > 0 in minor units. total_price is the sum of price * quantity.
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        request  body      order.CreateOrderInput  true  "Order items"
// @Success      201      {object}  order.Order
// @Failure      400      {object}  errorResponse  "INVALID_JSON or VALIDATION_ERROR"
// @Failure      413      {object}  errorResponse  "BODY_TOO_LARGE"
// @Failure      500      {object}  errorResponse  "INTERNAL_ERROR"
// @Router       /api/v1/orders [post]
func (h *api) createOrder(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeJSON[order.CreateOrderInput](w, r)
	if !ok {
		return
	}
	created, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// @Summary      List orders
// @Description  Returns all orders in creation order.
// @Tags         orders
// @Produce      json
// @Success      200  {array}   order.Order
// @Failure      500  {object}  errorResponse  "INTERNAL_ERROR"
// @Router       /api/v1/orders [get]
func (h *api) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.service.List(r.Context())
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// @Summary      Get order
// @Tags         orders
// @Produce      json
// @Param        id   path      int  true  "Order ID"  minimum(1)
// @Success      200  {object}  order.Order
// @Failure      400  {object}  errorResponse  "INVALID_ID"
// @Failure      404  {object}  errorResponse  "ORDER_NOT_FOUND"
// @Failure      500  {object}  errorResponse  "INTERNAL_ERROR"
// @Router       /api/v1/orders/{id} [get]
func (h *api) getOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	found, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

// @Summary      Change order status
// @Description  Allowed transitions: new -> processing, new -> cancelled, processing -> completed, processing -> cancelled. completed and cancelled are final.
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        id       path      int                      true  "Order ID"  minimum(1)
// @Param        request  body      order.UpdateStatusInput  true  "New status"
// @Success      200      {object}  order.Order
// @Failure      400      {object}  errorResponse  "INVALID_ID, INVALID_JSON or INVALID_STATUS"
// @Failure      404      {object}  errorResponse  "ORDER_NOT_FOUND"
// @Failure      413      {object}  errorResponse  "BODY_TOO_LARGE"
// @Failure      409      {object}  errorResponse  "INVALID_STATUS_TRANSITION"
// @Failure      500      {object}  errorResponse  "INTERNAL_ERROR"
// @Router       /api/v1/orders/{id}/status [patch]
func (h *api) updateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	input, ok := decodeJSON[order.UpdateStatusInput](w, r)
	if !ok {
		return
	}
	updated, err := h.service.UpdateStatus(r.Context(), id, input.Status)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// @Summary      Get status history
// @Description  Returns status changes of the order in chronological order.
// @Tags         orders
// @Produce      json
// @Param        id   path      int  true  "Order ID"  minimum(1)
// @Success      200  {array}   order.StatusChange
// @Failure      400  {object}  errorResponse  "INVALID_ID"
// @Failure      404  {object}  errorResponse  "ORDER_NOT_FOUND"
// @Failure      500  {object}  errorResponse  "INTERNAL_ERROR"
// @Router       /api/v1/orders/{id}/history [get]
func (h *api) orderHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	history, err := h.service.History(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		writeError(w, http.StatusBadRequest, codeInvalidID, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var zero T
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, codeBodyTooLarge,
			fmt.Sprintf("request body must not exceed %d MiB", maxBodyBytes>>20))
		return zero, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidJSON, "request body cannot be read")
		return zero, false
	}
	value, err := decodeObject[T](body)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidJSON, "request body must be a single valid JSON object")
		return zero, false
	}
	return value, true
}

func decodeObject[T any](body []byte) (T, error) {
	var value *T
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return *new(T), fmt.Errorf("decode body: %w", err)
	}
	if value == nil {
		return *new(T), errors.New("decode body: null instead of an object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return *new(T), errors.New("decode body: more than one JSON value")
	}
	return *value, nil
}
