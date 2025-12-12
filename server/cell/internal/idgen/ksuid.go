package idgen

import (
	"github.com/segmentio/ksuid"
)

// KSUIDGenerator creates KSUID-based IDs.
type KSUIDGenerator struct{}

// NewKSUIDGenerator returns a KSUID generator.
func NewKSUIDGenerator() *KSUIDGenerator {
	return &KSUIDGenerator{}
}

// NewID returns a new KSUID string.
func (KSUIDGenerator) NewID() (string, error) {
	id, err := ksuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
