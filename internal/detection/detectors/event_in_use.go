package detectors

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// Words of a controller that failed to delete an external resource
// because something still uses it, matched in lower case: the delete
// verb ("DeleteLoadBalancer", "failed to delete", "deprovisioning") and
// the refusal ("ResourceInUse", "is in use", "associated with another
// service", "DependencyViolation"). No controller or cloud is named;
// the pair of words is the rule.
var (
	deleteWords = regexp.MustCompile(`delete|deleting|deletion|destroy|` +
		`deprovision|teardown|tear down`)
	inUseWords = regexp.MustCompile(`resourceinuse|resource in use|` +
		`(is|still|currently) in use|in use by|associated with another|` +
		`dependencyviolation|has dependent|still attached`)
)

// inUseDeleteFailure reports an event message of a failed delete of a
// resource that is still in use. The controller retries it forever and
// nothing changes until someone frees or removes what uses it.
func inUseDeleteFailure(message string) bool {
	lower := strings.ToLower(message)
	return deleteWords.MatchString(lower) && inUseWords.MatchString(lower)
}

// inUseDeleteFinding is the finding of a controller's failed delete of
// a resource in use. It is reported at its first occurrence, as a
// warning: unlike an unknown event, its words already say it will not
// pass by itself. Like every event kwatch has no detector for, it waits
// for the digest. The count is the event's own, which the API server
// keeps across kwatch restarts, not only the quarter hour's. An object
// that is itself being deleted is left to its stuck deletion, which
// quotes the latest Warning event.
func inUseDeleteFinding(reason string, g *eventGroup) detection.Finding {
	count := max(g.count, g.newest.Count, 1)
	return detection.Finding{
		Reason:   reasons.UnusualEvent(reason),
		Severity: detection.Warning, Since: g.first,
		Mode: detection.ModeUnusualEvent,
		Summary: "A controller cannot delete an external resource " +
			"that is still in use (" + reason + ")",
		Evidence: []detection.Evidence{
			{Label: "event", Value: g.newest.Message},
			{Label: "occurrences", Value: strconv.Itoa(count)},
		},
	}
}
