package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/feature"
)

// ProChecksText is the Community line that counts, since kwatch
// started, the Pro findings it found and did not show. It is empty when
// there are none: nothing is sent then.
func ProChecksText(c feature.WithheldCounts) string {
	total := c.Total()
	if total == 0 {
		return ""
	}
	issues := " issues"
	if total == 1 {
		issues = " issue"
	}
	return "Pro checks found " + strconv.Itoa(total) + issues +
		" since kwatch started: " + c.Text() + ". Get Pro free during " +
		"early access to see them: kwatch.dev/pro"
}
