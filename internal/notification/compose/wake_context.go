package compose

import "strconv"

// wakeSentences say that the problem began while the cluster was waking
// up (see incident.WakeContext), as context: the wake-up held back the
// startup failures of its own pods, so a problem that is announced
// outlasted it or is not one it explains.
func wakeSentences(f caseFacts) []sentence {
	if f.wake == nil || f.wake.Started == 0 {
		return nil
	}
	verb := "was waking up"
	if f.wake.Ongoing {
		verb = "is waking up"
	}
	return []sentence{{part: partChanges, text: "The cluster " + verb +
		": " + strconv.Itoa(f.wake.Started) + " " + workloadsWord(
		f.wake.Started) + " started " + startedWhen(f.wake.From,
		f.wake.To) + "."}}
}

func workloadsWord(n int) string {
	if n == 1 {
		return "workload"
	}
	return "workloads"
}
