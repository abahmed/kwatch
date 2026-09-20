package app

import (
	"github.com/abahmed/kwatch/internal/metrics"
)

// onBaselineChange forwards the newest baseline snapshot to the saver.
func onBaselineChange(
	baselineCh chan map[string]map[string]int64,
) func(map[string]map[string]int64) {
	return func(b map[string]map[string]int64) {
		total := 0
		for _, pods := range b {
			total += len(pods)
		}
		metrics.DefaultRegistry().BaselineSize.Store(int64(total))
		select {
		case baselineCh <- b:
		default:
			// Channel full: drop oldest, keep newest.
			select {
			case <-baselineCh:
			default:
			}
			select {
			case baselineCh <- b:
			default:
			}
		}
	}
}
