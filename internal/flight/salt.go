package flight

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/abahmed/kwatch/internal/storage"
)

// saltKey holds the pseudonym salt in the state bucket.
const saltKey = "flight.salt"

// saltBytes is the size of the random salt.
const saltBytes = 16

// LoadSalt returns the pseudonym salt of the state file, creating a
// random one on first use. The salt stays in the state file: it keys the
// pseudonyms, so keeping it means a pod keeps its pseudonym across
// restarts and exports, and never exporting it means nobody can guess
// names back from an exported log.
func LoadSalt(s *storage.Store) (string, error) {
	values := storage.StateValues[string](s)
	salt, found, err := values.Get(saltKey)
	if err != nil {
		return "", err
	}
	if found && salt != "" {
		return salt, nil
	}
	raw := make([]byte, saltBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("flight: salt: %w", err)
	}
	salt = hex.EncodeToString(raw)
	return salt, values.Put(saltKey, salt)
}
