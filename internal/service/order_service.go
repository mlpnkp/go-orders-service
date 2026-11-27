package service

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"example.com/order-service/internal/order"
)

type OrderService struct {
	repository order.Repository
	now        func() time.Time
	mu         sync.Mutex
}

func NewOrderService(repository order.Repository, now func() time.Time) *OrderService {
	return &OrderService{repository: repository, now: now}
}

func (s *OrderService) Create(ctx context.Context, input order.CreateOrderInput) (order.Order, error) {
	total, err := totalPrice(input.Items)
	if err != nil {
		return order.Order{}, err
	}

	createdAt := s.now().UTC()
	created, err := s.repository.Create(ctx, order.Order{
		Items:      slices.Clone(input.Items),
		TotalPrice: total,
		Status:     order.StatusNew,
		CreatedAt:  createdAt,
		History:    []order.StatusChange{{Status: order.StatusNew, ChangedAt: createdAt}},
	})
	if err != nil {
		return order.Order{}, fmt.Errorf("create order: %w", err)
	}
	return created, nil
}

func (s *OrderService) GetByID(ctx context.Context, id int64) (order.Order, error) {
	found, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return order.Order{}, fmt.Errorf("get order %d: %w", id, err)
	}
	return found, nil
}

func (s *OrderService) List(ctx context.Context) ([]order.Order, error) {
	orders, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return orders, nil
}

func (s *OrderService) UpdateStatus(ctx context.Context, id int64, status order.Status) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return order.Order{}, fmt.Errorf("update order %d: %w", id, err)
	}
	if !status.Valid() {
		return order.Order{}, fmt.Errorf("update order %d to %q: %w", id, status, order.ErrInvalidStatus)
	}
	if !current.Status.CanTransitionTo(status) {
		return order.Order{}, fmt.Errorf("update order %d from %q to %q: %w",
			id, current.Status, status, order.ErrInvalidStatusTransition)
	}

	current.Status = status
	current.History = append(current.History,
		order.StatusChange{Status: status, ChangedAt: s.now().UTC()})

	updated, err := s.repository.Update(ctx, current)
	if err != nil {
		return order.Order{}, fmt.Errorf("update order %d: %w", id, err)
	}
	return updated, nil
}

func (s *OrderService) History(ctx context.Context, id int64) ([]order.StatusChange, error) {
	found, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get history of order %d: %w", id, err)
	}
	return found.History, nil
}

func totalPrice(items []order.Item) (int64, error) {
	if len(items) == 0 {
		return 0, &order.ValidationError{Field: "items", Reason: "must contain at least one item"}
	}

	var total int64
	for i, item := range items {
		if err := validateItem(i, item); err != nil {
			return 0, err
		}
		if item.Price > math.MaxInt64/int64(item.Quantity) || total > math.MaxInt64-item.Price*int64(item.Quantity) {
			return 0, &order.ValidationError{Field: "items", Reason: "total price is too large"}
		}
		total += item.Price * int64(item.Quantity)
	}
	return total, nil
}

func validateItem(i int, item order.Item) error {
	switch {
	case strings.TrimSpace(item.Name) == "":
		return &order.ValidationError{Field: fmt.Sprintf("items[%d].name", i), Reason: "must not be empty"}
	case item.Quantity <= 0:
		return &order.ValidationError{Field: fmt.Sprintf("items[%d].quantity", i), Reason: "must be greater than 0"}
	case item.Price <= 0:
		return &order.ValidationError{Field: fmt.Sprintf("items[%d].price", i), Reason: "must be greater than 0"}
	default:
		return nil
	}
}
