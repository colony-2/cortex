package idgen

import (
	"github.com/segmentio/ksuid"
)

// KSUIDGenerator generates KSUIDs for recipes.
type KSUIDGenerator struct{}

// NewKSUIDGenerator creates a new KSUID generator.
func NewKSUIDGenerator() *KSUIDGenerator {
	return &KSUIDGenerator{}
}

// NewID generates a new KSUID.
func (g *KSUIDGenerator) NewID() (string, error) {
	id := ksuid.New()
	return id.String(), nil
}
