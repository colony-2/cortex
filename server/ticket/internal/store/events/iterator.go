package events

import (
	"context"

	ticketstore "github.com/divisive-ai/vibethis/server/ticket/internal/store/tickets"
)

type sliceIterator[T any] struct {
	items  []T
	index  int
	closed bool
}

func newSliceIterator[T any](items []T) ticketstore.Iterator[T] {
	return &sliceIterator[T]{items: items}
}

func (it *sliceIterator[T]) Next(ctx context.Context) (T, error) {
	var zero T
	if it.closed {
		return zero, ticketstore.ErrIteratorDone
	}
	if it.index >= len(it.items) {
		return zero, ticketstore.ErrIteratorDone
	}
	item := it.items[it.index]
	it.index++
	return item, nil
}

func (it *sliceIterator[T]) Close(ctx context.Context) error {
	it.closed = true
	return nil
}
