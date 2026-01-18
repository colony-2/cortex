package tickets

import (
	"context"
	"errors"
)

type Iterator[T any] interface {
	Next(ctx context.Context) (T, error)
	Close(ctx context.Context) error
}

var ErrIteratorDone = errors.New("iterator: done")

type sliceIterator[T any] struct {
	items  []T
	index  int
	closed bool
}

func newSliceIterator[T any](items []T) Iterator[T] {
	return &sliceIterator[T]{items: items}
}

func NewSliceIterator[T any](items []T) Iterator[T] {
	return newSliceIterator(items)
}

func (it *sliceIterator[T]) Next(ctx context.Context) (T, error) {
	var zero T
	if it.closed {
		return zero, ErrIteratorDone
	}
	if it.index >= len(it.items) {
		return zero, ErrIteratorDone
	}
	item := it.items[it.index]
	it.index++
	return item, nil
}

func (it *sliceIterator[T]) Close(ctx context.Context) error {
	it.closed = true
	return nil
}
