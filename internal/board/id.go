package board

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// NewID returns a short random lowercase base32 id (8 chars, 40 bits).
func NewID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("kando: crypto/rand unavailable: " + err.Error())
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}
