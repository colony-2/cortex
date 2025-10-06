package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Invocation captures deterministic identifiers for an op execution.
type Invocation struct {
	RecipeID   string
	NodePath   string
	InvokeSeq  int
	BoxID      string
	ActivityID string
	ID         string
	// Deps carries runtime-scoped services; excluded from serialization/hashing.
	Deps ServiceDependencies2 `json:"-" yaml:"-" mapstructure:"-"`
}

// InvocationContext provides a legacy representation of Invocation for context binding APIs.
type InvocationContext struct {
	RecipeID   string
	NodePath   string
	InvokeSeq  int
	BoxID      string
	ActivityID string
	ID         string
	Deps       ServiceDependencies2 `json:"-" yaml:"-" mapstructure:"-"`
}

// InvocationContext returns a pointer to a context struct mirroring the invocation fields.
func (inv Invocation) InvocationContext() *InvocationContext {
	return &InvocationContext{
		RecipeID:   inv.RecipeID,
		NodePath:   inv.NodePath,
		InvokeSeq:  inv.InvokeSeq,
		BoxID:      inv.BoxID,
		ActivityID: inv.ActivityID,
		ID:         inv.ID,
		Deps:       inv.Deps,
	}
}

// Hash returns a truncated hex-encoded SHA-256 hash over deterministic invocation fields.
func (inv Invocation) Hash() string {
	hasher := sha256.New()
	hasher.Write([]byte(inv.RecipeID))
	hasher.Write([]byte{0})
	hasher.Write([]byte(inv.NodePath))
	hasher.Write([]byte{0})
	hasher.Write([]byte(strconv.Itoa(inv.InvokeSeq)))
	hasher.Write([]byte{0})
	hasher.Write([]byte(inv.BoxID))
	hasher.Write([]byte{0})
	hasher.Write([]byte(inv.ActivityID))

	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum[:8])
}
