package matrix

import (
	"crypto/rand"
	"math/big"
)

const roomIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM" +
	"NOPQRSTUVWXYZ0123456789"

// randomRoomIDPart creates an opaque local transaction identifier. Matrix
// uses it only for request idempotency; it is not part of alert identity.
func randomRoomIDPart(length int) string {
	if length <= 0 {
		return ""
	}
	result := make([]byte, length)
	limit := big.NewInt(int64(len(roomIDAlphabet)))
	for index := range result {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return ""
		}
		result[index] = roomIDAlphabet[n.Int64()]
	}
	return string(result)
}
