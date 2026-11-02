package compose

import (
	"regexp"
	"unicode/utf8"
)

// A generated name (a CSI VolumeAttachment is "csi-" and 64 hex digits)
// says nothing a reader can use and makes a line hard to read. Prose
// shows its start and end around an ellipsis, enough to tell it apart
// and to find it; commands keep the full name.
const (
	// maxProseName is the longest name prose shows whole: the length of
	// a DNS label, which most object names fit in.
	maxProseName = 63
	// proseNameHead and proseNameTail are how many characters of a
	// shortened name prose keeps at each end.
	proseNameHead = 12
	proseNameTail = 6
)

// hexRun is a run of hex digits as long as a SHA-256 digest is.
var hexRun = regexp.MustCompile(`[0-9a-f]{32,}`)

// proseName is name as prose shows it: whole, or, when it is longer
// than maxProseName or holds a hash, "csi-5468c63f…9d2e1a".
func proseName(name string) string {
	if utf8.RuneCountInString(name) <= maxProseName &&
		!hexRun.MatchString(name) {
		return name
	}
	runes := []rune(name)
	if len(runes) <= proseNameHead+proseNameTail+1 {
		return name
	}
	return string(runes[:proseNameHead]) + "…" +
		string(runes[len(runes)-proseNameTail:])
}
