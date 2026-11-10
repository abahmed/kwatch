package matrix

import (
	"encoding/json"

	"github.com/abahmed/kwatch/internal/notification"
)

// eventBodyLimit keeps the event under Matrix's 65536 byte cap for a
// whole event, which also holds the room, sender and signature fields.
const eventBodyLimit = 60000

// fitBodies cuts the plain and HTML bodies until both fit in one event.
// JSON escapes "<" as six bytes, so the budget shrinks until the
// marshalled pair is within eventBodyLimit.
func fitBodies(plain, formatted string) (string, string) {
	limit := eventBodyLimit / 2
	for limit > 1 {
		p := notification.Truncate(plain, limit)
		f := notification.TruncateHTML(formatted, limit)
		size, _ := json.Marshal([]string{p, f})
		if len(size) <= eventBodyLimit {
			return p, f
		}
		limit = limit * 3 / 4
	}
	return "", ""
}
