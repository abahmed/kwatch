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
	if i := firstTroubleIndex(lines); i >= 0 {
		return strings.TrimSpace(lines[i])
	}
	return ""
}

// isTroubleLine reports a line that names a failure and is not a stack
// frame.
func isTroubleLine(line string) bool {
	return IsErrorLine(line) && !stackFrame.MatchString(line)
}
