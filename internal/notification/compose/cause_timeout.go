package compose

import "regexp"

// timeoutAddress finds the address of a network timeout in the app's own
// error: "dial tcp 10.96.14.7:5432: i/o timeout".
var timeoutAddress = regexp.MustCompile(
	`dial tcp ([0-9a-zA-Z.:\[\]-]+): (?:i/o timeout|connect: connection ` +
		`timed out)`)

// timeoutSentence replaces "look at the app itself" when the app's error
// is a network timeout to an address and everything around it was
// checked: the app is not what to look at, the path to the address is.
func timeoutSentence(f caseFacts) (string, bool) {
	words := checkedWordsFor(f.p.Checked)
	if len(words) == 0 || !f.ok || !lookAtTheApp(f, false) {
		return "", false
	}
	// Only the lead's own error counts: other members of the incident
	// may time out for their own reasons.
	for _, e := range f.lead.Evidence {
		if g := timeoutAddress.FindStringSubmatch(e.Value); g != nil {
			return "No cause found in cluster state: its " +
				joinWords(words) + " " +
				verb(len(words), "is", "are") +
				" healthy and unchanged. Its error is a network " +
				"timeout to " + g[1] + ", so check the path to " +
				"that address.", true
		}
	}
	return "", false
}
