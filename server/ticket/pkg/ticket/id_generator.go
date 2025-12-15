package ticket

import "github.com/colony-2/colony2/server/ticket/internal/idgen"

const DefaultIDLength = idgen.DefaultIDLength

type Base58Generator = idgen.Base58Generator
type KSUIDGenerator = idgen.KSUIDGenerator

func NewBase58Generator(length int) *Base58Generator {
	return idgen.NewBase58Generator(length)
}

func NewKSUIDGenerator() *KSUIDGenerator {
	return idgen.NewKSUIDGenerator()
}
