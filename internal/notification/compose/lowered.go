package compose

// loweredSentences say, to the paging providers alone, that an incident
// that paged is no longer urgent: the previous version serves again and
// only the new rollout fails (incident.ReasonTierLowered). Like every
// message it names the cluster, through the subject.
func loweredSentences(f caseFacts) []sentence {
	subject := leadSubject(f)
	return []sentence{{part: partLead, text: endSentence(capitalName(
		subject, f.leadName(subject)+" is no longer urgent: the previous "+
			"version serves again; only the new rollout fails"))}}
}
