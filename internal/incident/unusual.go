package incident

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// unusual reports an incident with a member that restarts far more than
// its workload ever has: restarts from a workload that never restarts
// are news however often the incident was heard before, so they are
// never written off as known or rhythmic. A crash loop is left to those
// rules on purpose: a workload that crashes at the same time every day
// has made its crash routine, and its history of that crash says more
// than its restart rate.
func unusual(p *Incident) bool {
	for _, s := range p.Members {
		if s.Reason == reasons.HighRestartCount &&
			s.Normal == detection.NormalUnusual {
			return true
		}
	}
	return false
}
