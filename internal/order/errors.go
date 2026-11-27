package order

import (
	"errors"
	"fmt"
)

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrInvalidOrder            = errors.New("invalid order")
	ErrInvalidStatus           = errors.New("invalid status")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Reason)
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidOrder
}
