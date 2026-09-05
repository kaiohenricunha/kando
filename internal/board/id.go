package board

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// encodeID renders 5 bytes as the 8-char lowercase base32 id used everywhere.
func encodeID(b []byte) string {
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:5]))
}

// DeriveID returns the id of a card that has none written down, derived from
// seed instead of chance. The store calls it while parsing, so a file read
// twice yields the same ids both times: a page can render a link or a form
// for a hand-written card and the request it produces still finds that card.
// A card whose id reaches disk keeps it; this is only the fallback.
func DeriveID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return encodeID(sum[:])
}

// NewID returns a short random lowercase base32 id (8 chars, 40 bits).
func NewID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("kando: crypto/rand unavailable: " + err.Error())
	}
	return encodeID(b[:])
}
