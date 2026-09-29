package story

import (
	"fmt"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/notice"

	"github.com/abahmed/kwatch/internal/problem"
)

// maxSummaryLines bounds the startup summary.
const maxSummaryLines = 15

// StartupSummary is one message listing the problems that already existed
// when kwatch started, instead of one alert each.
func StartupSummary(
	decisions []problem.Decision, now time.Time,
) notice.Message {
	sort.SliceStable(decisions, func(i, j int) bool {
		return decisions[i].Problem.Tier > decisions[j].Problem.Tier
	})
	msg := notice.Message{
		Key:    "startup",
		Status: notice.StatusWarning,
		Title: fmt.Sprintf("kwatch started: %d existing problem(s) found",
			len(decisions)),
	}
	for i, d := range decisions {
		if i == maxSummaryLines {
			msg.Lines = append(msg.Lines, fmt.Sprintf("…and %d more",
				len(decisions)-maxSummaryLines))
			break
		}
		msg.Lines = append(msg.Lines, "• "+Write(d, now).Title)
	}
	msg.Lines = append(msg.Lines, "New changes to these problems are "+
		"reported as they happen.")
	return msg
}
