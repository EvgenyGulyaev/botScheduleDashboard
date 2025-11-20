package helpers

import (
	"context"
	"errors"
	"time"
)

func WithTimeout[T any](g func() T, sec int64) (T, error) {
	c, cancel := context.WithTimeout(context.Background(), time.Duration(sec)*time.Second)
	defer cancel()
	return call(c, g)
}

func call[T any](ctx context.Context, g func() T) (T, error) {
	result := make(chan T, 1)

	go func() {
		defer close(result)
		select {
		case result <- g():
		case <-ctx.Done():
		}
	}()

	select {
	case v := <-result:
		return v, nil
	case <-ctx.Done():
		return *new(T), errors.New("timeout error")
	}
}
