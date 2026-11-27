package order

import "time"

type Status string

const (
	StatusNew        Status = "new"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusProcessing, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

func (s Status) CanTransitionTo(next Status) bool {
	switch s {
	case StatusNew:
		return next == StatusProcessing || next == StatusCancelled
	case StatusProcessing:
		return next == StatusCompleted || next == StatusCancelled
	default:
		return false
	}
}

type Item struct {
	Name     string `json:"name" example:"Keyboard"`
	Quantity int    `json:"quantity" example:"1"`
	Price    int64  `json:"price" example:"500000"`
}

type StatusChange struct {
	Status    Status    `json:"status" enums:"new,processing,completed,cancelled" example:"new"`
	ChangedAt time.Time `json:"changed_at" example:"2026-11-20T10:00:00Z"`
}

type Order struct {
	ID         int64          `json:"id" example:"1"`
	Items      []Item         `json:"items"`
	TotalPrice int64          `json:"total_price" example:"500000"`
	Status     Status         `json:"status" enums:"new,processing,completed,cancelled" example:"new"`
	CreatedAt  time.Time      `json:"created_at" example:"2026-11-20T10:00:00Z"`
	History    []StatusChange `json:"-"`
}

type CreateOrderInput struct {
	Items []Item `json:"items"`
}

type UpdateStatusInput struct {
	Status Status `json:"status" enums:"new,processing,completed,cancelled" example:"processing"`
}
