package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"example.com/order-service/internal/order"
	"example.com/order-service/internal/repository/memory"
)

func TestOrderRepository(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewOrderRepository()

	first, err := repo.Create(ctx, order.Order{Items: []order.Item{{Name: "Keyboard", Quantity: 1, Price: 1}}})
	require.NoError(t, err)
	second, err := repo.Create(ctx, order.Order{Status: order.StatusNew})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, []int64{first.ID, second.ID})

	first.Items[0].Name = "Changed"
	stored, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, "Keyboard", stored.Items[0].Name)

	stored.Status = order.StatusProcessing
	_, err = repo.Update(ctx, stored)
	require.NoError(t, err)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, order.StatusProcessing, all[0].Status)
	assert.Equal(t, second.ID, all[1].ID)
}

func TestOrderRepositoryMissingOrder(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewOrderRepository()

	_, err := repo.GetByID(ctx, 42)
	require.ErrorIs(t, err, order.ErrOrderNotFound)

	_, err = repo.Update(ctx, order.Order{ID: 42})
	require.ErrorIs(t, err, order.ErrOrderNotFound)
}
