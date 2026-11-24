package kube

import (
	"regexp"
	"strings"
)

// logLevelKV finds a logfmt level field, logLevelJSON a JSON one.
var (
	logLevelKV = regexp.MustCompile(
		`(?i)(?:^|[\s{,])(?:level|lvl|severity)=["']?([a-z]+)`)
	logLevelJSON = regexp.MustCompile(
		`(?i)"(?:level|severity|lvl)"\s*:\s*"([a-z]+)"`)
	// logLevelKlog is the klog/glog prefix: I0102 15:04:05.
	logLevelKlog = regexp.MustCompile(`^([IWEF])\d{4}\s`)
	// logLevelToken is an upper-case level word at the start of a line,
	// after a leading timestamp, or in brackets anywhere.
	logLevelToken = regexp.MustCompile(
		`^(?:\d{4}[-/]\d\d[-/]\d\d[T ][\d:.,]+\S*\s+)?` +
			`[\[<]?(` + levelWords + `)[\]>:]?(?:\s|$)`)
	logLevelBracket = regexp.MustCompile(`\[\s*(` + levelWords + `)\s*\]`)
	// nullErrorField is an error field that says there was none.
	nullErrorField = regexp.MustCompile(
		`(?i)"?\b(?:error|err)"?\s*[=:]\s*` +
			`(?:null|<nil>|nil|none|""|'')(?:[\s,}]|$)`)
)

const levelWords = `TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERROR|ERR|` +
	`FATAL|CRITICAL|CRIT|PANIC|DPANIC|EMERG|ALERT|SEVERE`

// levelIsError maps a level word to whether it counts as an error and
// whether it is a level at all.
func levelIsError(word string) (isError, known bool) {
	switch strings.ToLower(word) {
	case "error", "err", "fatal", "panic", "dpanic", "critical", "crit",
		"emerg", "alert", "severe", "e", "f":
		return true, true
	case "trace", "debug", "info", "notice", "warn", "warning", "w", "i",
		"verbose", "dbg", "inf":
		return false, true
	}
	return false, false
}

// lineLevel reads the explicit level a line carries, if any.
func lineLevel(line string) (isError, found bool) {
	for _, re := range []*regexp.Regexp{logLevelKV, logLevelJSON,
		logLevelKlog, logLevelToken, logLevelBracket} {
		if m := re.FindStringSubmatch(line); m != nil {
			if isErr, known := levelIsError(m[1]); known {
				return isErr, true
			}
		}
	}
	return false, false
}

// IsSampledErrorLine reports whether a sampled log line is an error.
// A line with an explicit level counts only at error level or worse;
// one without keeps the word heuristic, minus error fields that are
// null or empty.
func IsSampledErrorLine(line string) bool {
	if isErr, found := lineLevel(line); found {
		return isErr
	}
	return IsErrorLine(nullErrorField.ReplaceAllString(line, " "))
}
