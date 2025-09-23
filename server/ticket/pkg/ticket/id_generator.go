package ticket

import "github.com/divisive-ai/vibethis/server/ticket/internal/idgen"

const DefaultIDLength = idgen.DefaultIDLength

type Base58Generator = idgen.Base58Generator

func NewBase58Generator(length int) *Base58Generator {
	return idgen.NewBase58Generator(length)
}
