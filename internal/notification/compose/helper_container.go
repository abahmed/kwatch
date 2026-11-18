package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// helperStates are the predicates summaryRewrites give an init or a
// sidecar container's finding, which leave out which container it is.
var helperStates = []string{
	"has a sidecar container that ", "has an init container that ",
}

// namedHelper names the container in a helper container's predicate:
// "has a sidecar container proxy that keeps crashing". A sidecar is an
// init container with restartPolicy Always that runs beside the app;
// naming it tells the reader which one of the pod's containers to read,
// and that it is not the app.
func namedHelper(id inventory.EntityID, state string) string {
	if id.Kind != kube.KindContainer {
		return state
	}
	_, name := splitContainer(id.Name)
	for _, prefix := range helperStates {
		if name != "" && strings.HasPrefix(state, prefix) {
			head := strings.TrimSuffix(prefix, "that ")
			return head + name + " that " + state[len(prefix):]
		}
	}
	return state
}

// helperSubject is what the "Cause not found yet" sentence tells the
// reader to look at: the sidecar or init container whose finding leads
// the message, else the app.
func helperSubject(summary string) string {
	for _, helper := range []string{"sidecar", "init container"} {
		if hasPrefixFold(summary, helper) {
			return "the " + helper
		}
	}
	return "the app"
}
