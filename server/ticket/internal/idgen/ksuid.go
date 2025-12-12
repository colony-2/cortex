package idgen

import "github.com/segmentio/ksuid"

// KSUIDGenerator produces ksuid strings.
type KSUIDGenerator struct{}

func NewKSUIDGenerator() *KSUIDGenerator { return &KSUIDGenerator{} }

func (KSUIDGenerator) NewID() (string, error) {
	id, err := ksuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
