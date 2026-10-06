package github

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// maxWaitHint caps a wait read from a message, so odd text cannot park
// delivery for long.
const maxWaitHint = 15 * time.Minute

// waitPattern finds a wait the message names, such as "retry after 60
// seconds" or "wait 2 minutes". The sender does not expose the
// Retry-After header of a 403, so the text is the only place a wait can
// be read from; without one the shared default backoff applies.
var waitPattern = regexp.MustCompile(
	`(?i)(?:retry|try again|wait)[^0-9]{0,30}(\d+)\s*(second|minute)`)

// waitHint returns the wait a rate-limit answer names, or 0. It looks in
// the StatusError summary first and then in the response body.
func waitHint(err error, body []byte) time.Duration {
	text := string(body)
	var status *transport.StatusError
	if errors.As(err, &status) {
		text = status.Body + " " + text
	}
	m := waitPattern.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return 0
	}
	n, convErr := strconv.Atoi(m[1])
	if convErr != nil || n <= 0 {
		return 0
	}
	wait := time.Duration(n) * time.Second
	if strings.EqualFold(m[2], "minute") {
		wait = time.Duration(n) * time.Minute
	}
	if wait > maxWaitHint {
		wait = maxWaitHint
	}
	return wait
}
