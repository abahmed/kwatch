package kube

import (
	"strconv"
	"strings"
)

// captureHeadLines is how many lines from the start of the trouble a
// bounded capture keeps: the first error and the lines right after it,
// such as the top of a panic's trace.
const captureHeadLines = 8

// Skipped-lines marker: "… 12 lines skipped …". It stands between the
// start of the trouble and the most recent lines of a capture.
const (
	skippedPrefix = "… "
	skippedSuffix = " lines skipped …"
)

// CapturedLines bounds captured lines, oldest first, to at most limit.
// Lines that fit are kept as they are. Otherwise the start of the
// trouble is kept: the first line that names a failure and is not a
// stack frame (the rule of FirstErrorLine), and up to captureHeadLines
// lines from it. Then comes a marker that counts the lines left out
// ("… 12 lines skipped …"), then the most recent lines. Output before
// the first error is dropped. Lines without any error keep only the
// most recent limit lines.
//
// Lines that already went through it keep their start: only the part
// after the marker moves on, and the marker's count grows. So an
// incident that captures more output over time still shows how its
// trouble began.
func CapturedLines(lines []string, limit int) []string {
	if len(lines) <= limit || limit <= 0 {
		return lines
	}
	head, skipped, rest, marked := splitCapture(lines)
	if !marked {
		start := firstTroubleIndex(lines)
		// Without an error, or with the first one among the most
		// recent lines, the most recent lines tell it all.
		if start < 0 || len(lines)-start <= limit || limit < 3 {
			return lines[len(lines)-limit:]
		}
		end := start + min(captureHeadLines, limit/2)
		head, rest = lines[start:end], lines[end:]
	}
	if keep := min(captureHeadLines, limit/2); len(head) > keep {
		// The lines cut from the end of the start sit next to the marker.
		skipped += len(head) - keep
		head = head[:keep]
	}
	room := max(limit-len(head)-1, 0)
	cut := max(len(rest)-room, 0)
	out := make([]string, 0, len(head)+1+len(rest)-cut)
	out = append(out, head...)
	out = append(out, skippedMarker(skipped+cut))
	return append(out, rest[cut:]...)
}

// firstTroubleIndex is the index of the first line FirstErrorLine would
// choose, or -1.
func firstTroubleIndex(lines []string) int {
	for i, line := range lines {
		if isTroubleLine(strings.TrimSpace(line)) {
			return i
		}
	}
	return -1
}

// splitCapture splits lines at the first skipped-lines marker.
func splitCapture(lines []string) (
	head []string, skipped int, rest []string, marked bool,
) {
	for i, line := range lines {
		if n, ok := parseSkipped(line); ok {
			return lines[:i], n, lines[i+1:], true
		}
	}
	return nil, 0, lines, false
}

func skippedMarker(n int) string {
	return skippedPrefix + strconv.Itoa(n) + skippedSuffix
}

// parseSkipped reads the count of a skipped-lines marker.
func parseSkipped(line string) (int, bool) {
	rest, ok := strings.CutPrefix(line, skippedPrefix)
	if !ok {
		return 0, false
	}
	count, ok := strings.CutSuffix(rest, skippedSuffix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(count)
	return n, err == nil && n >= 0
}
