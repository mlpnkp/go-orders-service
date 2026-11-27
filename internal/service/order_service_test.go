package service_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"example.com/order-service/internal/order"
	"example.com/order-service/internal/repository/memory"
	"example.com/order-service/internal/service"
)

var (
	errStorage = errors.New("storage is down")
	startTime  = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
)

type fakeRepository struct {
	*memory.OrderRepository
	createErr error
	getErr    error
	listErr   error
	updateErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{OrderRepository: memory.NewOrderRepository()}
}

func (r *fakeRepository) Create(ctx context.Context, value order.Order) (order.Order, error) {
	if r.createErr != nil {
		return order.Order{}, r.createErr
	}
	return r.OrderRepository.Create(ctx, value)
}

func (r *fakeRepository) GetByID(ctx context.Context, id int64) (order.Order, error) {
	if r.getErr != nil {
		return order.Order{}, r.getErr
	}
	return r.OrderRepository.GetByID(ctx, id)
}

func (r *fakeRepository) List(ctx context.Context) ([]order.Order, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.OrderRepository.List(ctx)
}

func (r *fakeRepository) Update(ctx context.Context, value order.Order) (order.Order, error) {
	if r.updateErr != nil {
		return order.Order{}, r.updateErr
	}
	return r.OrderRepository.Update(ctx, value)
}

func (r *fakeRepository) stored(t *testing.T, id int64) order.Order {
	t.Helper()
	value, err := r.OrderRepository.GetByID(context.Background(), id)
	require.NoError(t, err)
	return value
}

type stepClock struct {
	next time.Time
}

func (c *stepClock) Now() time.Time {
	now := c.next
	c.next = c.next.Add(time.Minute)
	return now
}

func newService(repo order.Repository) *service.OrderService {
	clock := &stepClock{next: startTime}
	return service.NewOrderService(repo, clock.Now)
}

func keyboard() order.Item {
	return order.Item{Name: "Keyboard", Quantity: 1, Price: 500000}
}

func createKeyboardOrder(t *testing.T, svc *service.OrderService) order.Order {
	t.Helper()
	created, err := svc.Create(context.Background(), order.CreateOrderInput{Items: []order.Item{keyboard()}})
	require.NoError(t, err)
	return created
}

func TestOrderServiceCreate(t *testing.T) {
	tests := []struct {
		name      string
		items     []order.Item
		createErr error
		wantTotal int64
		wantField string
	}{
		{name: "one item", items: []order.Item{keyboard()}, wantTotal: 500000},
		{
			name:      "total is sum of price times quantity",
			items:     []order.Item{keyboard(), {Name: "Mouse", Quantity: 3, Price: 150000}},
			wantTotal: 500000 + 3*150000,
		},
		{name: "nil items", items: nil, wantField: "items"},
		{name: "empty items", items: []order.Item{}, wantField: "items"},
		{name: "empty name", items: []order.Item{{Name: "", Quantity: 1, Price: 1}}, wantField: "items[0].name"},
		{name: "blank name", items: []order.Item{keyboard(), {Name: "  ", Quantity: 1, Price: 1}}, wantField: "items[1].name"},
		{name: "zero quantity", items: []order.Item{{Name: "Mouse", Quantity: 0, Price: 1}}, wantField: "items[0].quantity"},
		{name: "negative quantity", items: []order.Item{{Name: "Mouse", Quantity: -2, Price: 1}}, wantField: "items[0].quantity"},
		{name: "zero price", items: []order.Item{{Name: "Mouse", Quantity: 1, Price: 0}}, wantField: "items[0].price"},
		{name: "negative price", items: []order.Item{{Name: "Mouse", Quantity: 1, Price: -100}}, wantField: "items[0].price"},
		{name: "item total overflows int64", items: []order.Item{{Name: "Gold", Quantity: 2, Price: math.MaxInt64}}, wantField: "items"},
		{name: "order total overflows int64", items: []order.Item{{Name: "Gold", Quantity: 1, Price: math.MaxInt64}, keyboard()}, wantField: "items"},
		{name: "repository error", items: []order.Item{keyboard()}, createErr: errStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.createErr = tt.createErr
			svc := newService(repo)

			created, err := svc.Create(context.Background(), order.CreateOrderInput{Items: tt.items})

			if tt.wantField != "" {
				require.ErrorIs(t, err, order.ErrInvalidOrder)
				var validationErr *order.ValidationError
				require.ErrorAs(t, err, &validationErr)
				assert.Equal(t, tt.wantField, validationErr.Field)
				assert.Empty(t, mustList(t, repo))
				return
			}
			if tt.createErr != nil {
				require.ErrorIs(t, err, tt.createErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, int64(1), created.ID)
			assert.Equal(t, tt.items, created.Items)
			assert.Equal(t, tt.wantTotal, created.TotalPrice)
			assert.Equal(t, order.StatusNew, created.Status)
			assert.Equal(t, startTime, created.CreatedAt)
			assert.Equal(t, []order.StatusChange{{Status: order.StatusNew, ChangedAt: startTime}}, created.History)
			assert.Equal(t, created, repo.stored(t, created.ID))
		})
	}
}

func TestOrderServiceCreateCopiesInputItems(t *testing.T) {
	items := []order.Item{keyboard()}

	created, err := newService(newFakeRepository()).Create(context.Background(), order.CreateOrderInput{Items: items})
	require.NoError(t, err)

	items[0].Name = "Changed"
	assert.Equal(t, "Keyboard", created.Items[0].Name)
}

func TestOrderServiceGetByID(t *testing.T) {
	tests := []struct {
		name    string
		id      int64
		getErr  error
		wantErr error
	}{
		{name: "existing order", id: 1},
		{name: "missing order", id: 42, wantErr: order.ErrOrderNotFound},
		{name: "repository error", id: 1, getErr: errStorage, wantErr: errStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := newService(repo)
			created := createKeyboardOrder(t, svc)
			repo.getErr = tt.getErr

			got, err := svc.GetByID(context.Background(), tt.id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, created, got)
		})
	}
}

func TestOrderServiceList(t *testing.T) {
	tests := []struct {
		name    string
		create  int
		listErr error
	}{
		{name: "no orders", create: 0},
		{name: "orders in creation order", create: 3},
		{name: "repository error", create: 1, listErr: errStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := newService(repo)
			for range tt.create {
				createKeyboardOrder(t, svc)
			}
			repo.listErr = tt.listErr

			got, err := svc.List(context.Background())

			if tt.listErr != nil {
				require.ErrorIs(t, err, tt.listErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, got, tt.create)
			for i, o := range got {
				assert.Equal(t, int64(i+1), o.ID)
			}
		})
	}
}

func TestOrderServiceUpdateStatus(t *testing.T) {
	processing := []order.Status{order.StatusProcessing}
	completed := []order.Status{order.StatusProcessing, order.StatusCompleted}
	cancelled := []order.Status{order.StatusCancelled}

	tests := []struct {
		name    string
		path    []order.Status
		next    order.Status
		wantErr error
	}{
		{name: "new to processing", next: order.StatusProcessing},
		{name: "new to cancelled", next: order.StatusCancelled},
		{name: "processing to completed", path: processing, next: order.StatusCompleted},
		{name: "processing to cancelled", path: processing, next: order.StatusCancelled},
		{name: "new to completed", next: order.StatusCompleted, wantErr: order.ErrInvalidStatusTransition},
		{name: "new to new", next: order.StatusNew, wantErr: order.ErrInvalidStatusTransition},
		{name: "processing to new", path: processing, next: order.StatusNew, wantErr: order.ErrInvalidStatusTransition},
		{name: "processing to processing", path: processing, next: order.StatusProcessing, wantErr: order.ErrInvalidStatusTransition},
		{name: "completed to cancelled", path: completed, next: order.StatusCancelled, wantErr: order.ErrInvalidStatusTransition},
		{name: "completed to processing", path: completed, next: order.StatusProcessing, wantErr: order.ErrInvalidStatusTransition},
		{name: "cancelled to processing", path: cancelled, next: order.StatusProcessing, wantErr: order.ErrInvalidStatusTransition},
		{name: "cancelled to completed", path: cancelled, next: order.StatusCompleted, wantErr: order.ErrInvalidStatusTransition},
		{name: "unknown status", next: order.Status("shipped"), wantErr: order.ErrInvalidStatus},
		{name: "empty status", next: order.Status(""), wantErr: order.ErrInvalidStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := newService(repo)
			created := createKeyboardOrder(t, svc)
			for _, status := range tt.path {
				_, err := svc.UpdateStatus(context.Background(), created.ID, status)
				require.NoError(t, err)
			}
			before := repo.stored(t, created.ID)

			updated, err := svc.UpdateStatus(context.Background(), created.ID, tt.next)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, before, repo.stored(t, created.ID))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.next, updated.Status)
			assert.Equal(t, updated, repo.stored(t, created.ID))
			require.Len(t, updated.History, len(before.History)+1)
			assert.Equal(t, tt.next, updated.History[len(updated.History)-1].Status)
		})
	}
}

func TestOrderServiceUpdateStatusRepositoryErrors(t *testing.T) {
	tests := []struct {
		name      string
		id        int64
		status    order.Status
		getErr    error
		updateErr error
		wantErr   error
	}{
		{name: "missing order", id: 42, status: order.StatusProcessing, wantErr: order.ErrOrderNotFound},
		{name: "missing order with unknown status", id: 42, status: "bogus", wantErr: order.ErrOrderNotFound},
		{name: "get fails", id: 1, status: order.StatusProcessing, getErr: errStorage, wantErr: errStorage},
		{name: "update fails", id: 1, status: order.StatusProcessing, updateErr: errStorage, wantErr: errStorage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := newService(repo)
			createKeyboardOrder(t, svc)
			repo.getErr, repo.updateErr = tt.getErr, tt.updateErr

			_, err := svc.UpdateStatus(context.Background(), tt.id, tt.status)

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestOrderServiceHistory(t *testing.T) {
	tests := []struct {
		name string
		path []order.Status
		want []order.StatusChange
	}{
		{
			name: "new order has one entry",
			want: []order.StatusChange{{Status: order.StatusNew, ChangedAt: startTime}},
		},
		{
			name: "entries are appended in chronological order",
			path: []order.Status{order.StatusProcessing, order.StatusCompleted},
			want: []order.StatusChange{
				{Status: order.StatusNew, ChangedAt: startTime},
				{Status: order.StatusProcessing, ChangedAt: startTime.Add(time.Minute)},
				{Status: order.StatusCompleted, ChangedAt: startTime.Add(2 * time.Minute)},
			},
		},
		{
			name: "rejected transition adds no entry",
			path: []order.Status{order.StatusCancelled, order.StatusProcessing},
			want: []order.StatusChange{
				{Status: order.StatusNew, ChangedAt: startTime},
				{Status: order.StatusCancelled, ChangedAt: startTime.Add(time.Minute)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(newFakeRepository())
			created := createKeyboardOrder(t, svc)
			for _, status := range tt.path {
				_, _ = svc.UpdateStatus(context.Background(), created.ID, status)
			}

			got, err := svc.History(context.Background(), created.ID)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOrderServiceHistoryMissingOrder(t *testing.T) {
	_, err := newService(newFakeRepository()).History(context.Background(), 42)

	require.ErrorIs(t, err, order.ErrOrderNotFound)
}

func mustList(t *testing.T, repo *fakeRepository) []order.Order {
	t.Helper()
	orders, err := repo.OrderRepository.List(context.Background())
	require.NoError(t, err)
	return orders
}
