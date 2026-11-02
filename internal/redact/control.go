package redact

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// terminalEscapes match ANSI CSI sequences (colours, cursor moves), OSC
// sequences (window titles, hyperlinks) ended by BEL or ST, and any other
// two-byte escape. They are removed before redaction: a colour code
// between a key and its value would otherwise hide the pair from every
// key-name rule.
var terminalEscapes = regexp.MustCompile(
	`(?:\x1b\[|\x{9b})[0-9;?]*[ -/]*[@-~]|` +
		`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|` +
		`\x1b[@-_]?`,
)

// StripControl removes terminal escape sequences and control characters
// other than tab and newline, and resolves carriage returns the way a
// terminal shows them: a progress bar "10%\r20%\rERROR x" keeps only the
// text after the last carriage return of its line. A trailing carriage
// return (CRLF line ends) is dropped. Text without control characters is
// returned unchanged without allocating.
func StripControl(value string) string {
	if !hasControl(value) {
		return value
	}
	value = terminalEscapes.ReplaceAllString(value, "")
	if strings.IndexByte(value, '\r') >= 0 {
		value = resolveCarriageReturns(value)
	}
	return strings.Map(dropControl, value)
}

// hasControl reports whether value holds a byte or rune StripControl
// would change.
func hasControl(value string) bool {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b == '\t' || b == '\n' {
			continue
		}
		if b < 0x20 || b == 0x7f {
			return true
		}
		if b >= utf8.RuneSelf {
			return hasC1(value[i:])
		}
	}
	return false
}

// hasC1 reports whether value holds a C1 control rune (U+0080-U+009F),
// which some terminals read as an escape introducer, or a C0 control.
func hasC1(value string) bool {
	for _, r := range value {
		if isControlRune(r) {
			return true
		}
	}
	return false
}

func isControlRune(r rune) bool {
	if r == '\t' || r == '\n' {
		return false
	}
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

func dropControl(r rune) rune {
	if isControlRune(r) {
		return -1
	}
	return r
}

// resolveCarriageReturns keeps, on each line, the last non-blank segment
// after a carriage return: what a terminal would leave on screen.
func resolveCarriageReturns(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.IndexByte(line, '\r') >= 0 {
			line = lastSegment(line)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func lastSegment(line string) string {
	parts := strings.Split(line, "\r")
	for k := len(parts) - 1; k >= 0; k-- {
		if strings.TrimSpace(parts[k]) != "" {
			return parts[k]
		}
	}
	return ""
}
