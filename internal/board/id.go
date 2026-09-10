package board

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
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
// A card whose id reaches disk keeps it, unless another card already holds the
// same id; see DeriveFreeID.
func DeriveID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return encodeID(sum[:])
}

// DeriveFreeID is DeriveID for a card whose derived id may already belong to
// another card in the same file. When DeriveID(seed) is free it returns exactly
// that, so every id derived before shared ids were repaired stays byte-identical.
//
// Otherwise it salts the seed's digest, not the seed. A seed carries the card's
// whole notes, and re-hashing it on every retry would let a crafted file — one
// card with large notes, followed by cards whose written ids are its precomputed
// salt chain — make a single parse cost O(retries x notes). Salting the digest
// makes each retry constant-time.
func DeriveFreeID(seed string, taken func(string) bool) string {
	sum := sha256.Sum256([]byte(seed))
	id := encodeID(sum[:])
	var buf [sha256.Size + 8]byte
	copy(buf[:], sum[:])
	for salt := uint64(1); taken(id); salt++ {
		binary.BigEndian.PutUint64(buf[sha256.Size:], salt)
		next := sha256.Sum256(buf[:])
		id = encodeID(next[:])
	}
	return id
}

// NewID returns a short random lowercase base32 id (8 chars, 40 bits).
func NewID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("kando: crypto/rand unavailable: " + err.Error())
	}
	return encodeID(b[:])
}
