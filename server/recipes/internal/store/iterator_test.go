package store

import (
	"context"
	"errors"
	"testing"
)

func TestSliceIterator(t *testing.T) {
	items := []string{"a", "b", "c"}
	iter := newSliceIterator(items)

	ctx := context.Background()

	// Read all items
	for i, want := range items {
		got, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() at index %d failed: %v", i, err)
		}
		if got != want {
			t.Errorf("Next() at index %d = %q, want %q", i, got, want)
		}
	}

	// Should return ErrIteratorDone after exhausted
	_, err := iter.Next(ctx)
	if !errors.Is(err, ErrIteratorDone) {
		t.Errorf("Next() after exhausted = %v, want ErrIteratorDone", err)
	}
}

func TestSliceIterator_Empty(t *testing.T) {
	iter := newSliceIterator([]string{})
	ctx := context.Background()

	// Should immediately return ErrIteratorDone
	_, err := iter.Next(ctx)
	if !errors.Is(err, ErrIteratorDone) {
		t.Errorf("Next() on empty = %v, want ErrIteratorDone", err)
	}
}

func TestSliceIterator_Close(t *testing.T) {
	items := []string{"a", "b", "c"}
	iter := newSliceIterator(items)
	ctx := context.Background()

	// Read one item
	_, err := iter.Next(ctx)
	if err != nil {
		t.Fatalf("Next() failed: %v", err)
	}

	// Close iterator
	if err := iter.Close(ctx); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Should return ErrIteratorDone after closed
	_, err = iter.Next(ctx)
	if !errors.Is(err, ErrIteratorDone) {
		t.Errorf("Next() after Close() = %v, want ErrIteratorDone", err)
	}

	// Multiple Close calls should be safe
	if err := iter.Close(ctx); err != nil {
		t.Errorf("Second Close() failed: %v", err)
	}
}

func TestSliceIterator_Types(t *testing.T) {
	// Test with different types
	t.Run("int", func(t *testing.T) {
		iter := newSliceIterator([]int{1, 2, 3})
		ctx := context.Background()

		got, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() failed: %v", err)
		}
		if got != 1 {
			t.Errorf("Next() = %d, want 1", got)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type testStruct struct {
			Value string
		}
		items := []testStruct{{Value: "test"}}
		iter := newSliceIterator(items)
		ctx := context.Background()

		got, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() failed: %v", err)
		}
		if got.Value != "test" {
			t.Errorf("Next().Value = %q, want \"test\"", got.Value)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		s1 := "test1"
		s2 := "test2"
		items := []*string{&s1, &s2}
		iter := newSliceIterator(items)
		ctx := context.Background()

		got, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Next() failed: %v", err)
		}
		if *got != "test1" {
			t.Errorf("*Next() = %q, want \"test1\"", *got)
		}
	})
}
