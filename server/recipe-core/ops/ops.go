package ops

import (
	"sync"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
)

// opsRegistry is our singleton instance
type opsRegistry struct {
	ops sync.Map // No manual locking needed!
}

// singleton instance
var (
	instance *opsRegistry
	once     sync.Once
)

// getInstance returns the singleton instance of opsRegistry
func getInstance() *opsRegistry {
	once.Do(func() {
		instance = &opsRegistry{}
	})
	return instance
}

// Register adds a new operatio(s)) to the registry
func Register(ops ...types.RegisterableOp) {
	registry := getInstance()
	for _, op := range ops {
		registry.ops.Store(op.GetName(), op)
	}
}

// Get retrieves an operation by name with existence check
func Get(name string) (types.RegisterableOp, bool) {
	registry := getInstance()

	value, exists := registry.ops.Load(name)
	if exists {
		return value.(types.RegisterableOp), true
	}
	return nil, false
}

// List returns all registered operation names
func List() []types.RegisterableOp {
	registry := getInstance()

	ops := make([]types.RegisterableOp, 0, 10)
	registry.ops.Range(func(key, value any) bool {
		ops = append(ops, value.(types.RegisterableOp))
		return true // continue iteration
	})
	return ops
}

// Clear removes all operations from the registry
func Clear() {
	registry := getInstance()

	// sync.Map doesn't have a Clear method, so we need to delete each key
	registry.ops.Range(func(key, value any) bool {
		registry.ops.Delete(key)
		return true
	})
}

// Size returns the number of registered operations
func Size() int {
	registry := getInstance()

	count := 0
	registry.ops.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}
