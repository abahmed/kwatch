package notification

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// capHashLen is how many hex characters of the key's hash a capped key
// ends with.
const capHashLen = 12

// AlertKeyMax is AlertKey for a provider that accepts at most limit
// bytes of key. A key within the limit is returned unchanged. A longer one
// keeps a readable prefix and ends in a short hash of the whole key, so
// it stays unique and the same incident always gets the same key: the
// resolve finds the alert its trigger opened.
func (m Message) AlertKeyMax(cluster string, limit int) string {
	return capKey(m.AlertKey(cluster), limit)
}

// PageKeySuffix ends the key of the alert a pager opens when an incident
// it heard below the page tier rises to it (Message.RaisesPage).
const PageKeySuffix = "-page"

// AlertKeyVariant is AlertKeyMax with suffix appended. A key too long for
// limit is capped first, with room kept for suffix, so the variant still
// ends in suffix, differs from the plain key and is the same for every
// message of the incident. A limit of zero or less does not cap.
func (m Message) AlertKeyVariant(
	cluster, suffix string, limit int,
) string {
	key := m.AlertKey(cluster)
	room := limit - len(suffix)
	if limit <= 0 || len(key)+len(suffix) <= limit {
		return key + suffix
	}
	if room <= capHashLen {
		return capKey(key+suffix, limit)
	}
	return capKey(key, room) + suffix
}

func capKey(key string, limit int) string {
	if limit <= 0 || len(key) <= limit {
		return key
	}
	sum := sha256.Sum256([]byte(key))
	tail := "-" + hex.EncodeToString(sum[:])[:capHashLen]
	room := limit - len(tail)
	if room <= 0 {
		return strings.TrimPrefix(tail, "-")
	}
	cut := room
	for cut > 0 && !utf8.RuneStart(key[cut]) {
		cut--
	}
	return key[:cut] + tail
}
