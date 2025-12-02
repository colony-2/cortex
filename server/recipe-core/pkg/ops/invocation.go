package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Invocation captures deterministic identifiers for an op execution.
type Invocation struct {
	NodePath  string
	InvokeSeq int
}

// Hash returns a truncated hex-encoded SHA-256 hash over deterministic invocation fields.
func (inv Invocation) Hash() string {
	hasher := sha256.New()
	hasher.Write([]byte(inv.NodePath))
	hasher.Write([]byte{0})
	hasher.Write([]byte(strconv.Itoa(inv.InvokeSeq)))
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum[:8])
}
