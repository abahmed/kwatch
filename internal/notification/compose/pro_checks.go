package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/feature"
)

// ProChecksText is the Community line that counts, since kwatch
// started, the Pro findings it found and did not show. It is empty when
// there are none: nothing is sent then.
func ProChecksText(c feature.WithheldCounts) string {
	return proChecksLine(c, " since kwatch started: ")
}

// ProChecksTodayText is the daily form of ProChecksText: it counts the
// Pro findings first found today. It is empty when there are none.
func ProChecksTodayText(c feature.WithheldCounts) string {
	return proChecksLine(c, " today: ")
}

// proChecksLine is "Pro checks found 3 issues<when>2 readiness probes,
// 1 shared cause. Get Pro ...", or nothing when c counts nothing.
func proChecksLine(c feature.WithheldCounts, when string) string {
	total := c.Total()
	if total == 0 {
		return ""
	}
	issues := " issues"
	if total == 1 {
		issues = " issue"
	}
	return "Pro checks found " + strconv.Itoa(total) + issues + when +
		c.Text() + ". Get Pro free during " +
		"early access to see them: kwatch.dev/pro"
}
