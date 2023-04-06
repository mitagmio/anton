package rndm

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/tonindexer/anton/internal/addr"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func String(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func Bytes(l int) []byte {
	token := make([]byte, l)
	rand.Read(token) // nolint
	return token
}

func Address() *addr.Address {
	a, err := new(addr.Address).FromString(fmt.Sprintf("0:%x", Bytes(32)))
	if err != nil {
		panic(err)
	}
	return a
}
