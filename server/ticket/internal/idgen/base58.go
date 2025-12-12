package idgen

import (
	"crypto/rand"
	"math/big"
)

const DefaultIDLength = 27

var base58Alphabet = []rune("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")

type Base58Generator struct {
	length   int
	alphabet []rune
}

func NewBase58Generator(length int) *Base58Generator {
	if length <= 0 {
		length = DefaultIDLength
	}
	return &Base58Generator{length: length, alphabet: base58Alphabet}
}

func (g *Base58Generator) NewID() (string, error) {
	length := DefaultIDLength
	alphabet := base58Alphabet
	if g != nil {
		if g.length > 0 {
			length = g.length
		}
		if len(g.alphabet) > 0 {
			alphabet = g.alphabet
		}
	}
	result := make([]rune, length)
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		result[i] = alphabet[n.Int64()]
	}
	return string(result), nil
}
