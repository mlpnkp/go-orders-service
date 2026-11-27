package memory

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"example.com/order-service/internal/order"
)

type OrderRepository struct {
	mu    sync.RWMutex
	items []order.Order
}

func NewOrderRepository() *OrderRepository {
	return &OrderRepository{}
}

func (r *OrderRepository) Create(_ context.Context, value order.Order) (order.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	value.ID = int64(len(r.items)) + 1
	r.items = append(r.items, clone(value))
	return value, nil
}

func (r *OrderRepository) GetByID(_ context.Context, id int64) (order.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	i, err := r.index(id)
	if err != nil {
		return order.Order{}, err
	}
	return clone(r.items[i]), nil
}

func (r *OrderRepository) List(_ context.Context) ([]order.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]order.Order, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, clone(item))
	}
	return result, nil
}

func (r *OrderRepository) Update(_ context.Context, value order.Order) (order.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	i, err := r.index(value.ID)
	if err != nil {
		return order.Order{}, err
	}
	r.items[i] = clone(value)
	return value, nil
}

func (r *OrderRepository) index(id int64) (int, error) {
	if id < 1 || id > int64(len(r.items)) {
		return 0, fmt.Errorf("order %d: %w", id, order.ErrOrderNotFound)
	}
	return int(id - 1), nil
}

func clone(o order.Order) order.Order {
	o.Items = slices.Clone(o.Items)
	o.History = slices.Clone(o.History)
	return o
}
