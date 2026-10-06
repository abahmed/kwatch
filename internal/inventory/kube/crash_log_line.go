package kube

import (
	"regexp"
	"strings"
)

// stackFrame matches lines that only say where an error happened: Java
// and JavaScript "at" frames, Python "File" lines, Go goroutine headers
// and source positions, and elided-frame markers.
var stackFrame = regexp.MustCompile(`^(at \S+|File ".*", line \d+|` +
	`goroutine \d+ \[|\S*\.(go|py|js|ts|java|rb|rs):\d+|` +
	`\.\.\. \d+ more|Traceback \(most recent call last\)|#\d+ )`)

// FirstErrorLine is the first line that names a failure and is not a
// stack frame. The first error is the cause; the last frame of a trace
// only says where the program noticed it. The line is quoted, never
// interpreted. internal/pipeline calls this function too, so the log
// line a message quotes is chosen in one place.
func FirstErrorLine(lines []string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if IsErrorLine(line) && !stackFrame.MatchString(line) {
			return line
		}
	}
	return ""
}
