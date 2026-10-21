package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// secretMentioned is a crash caused by a Secret whose error text names
// it; the proof says how many of the errors do.
func secretMentioned(count, total int, errorText string) incident.Incident {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	secret := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
	crash := detection.Finding{
		Entity: inventory.CoreID(kube.KindContainer, "shop", "api-1-a/app"),
		Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
		Since: at(0, 0), Summary: "Container is crash looping"}
	if errorText != "" {
		crash.Evidence = []detection.Evidence{{Label: "error",
			Value: errorText}}
	}
	return incident.Incident{ID: "inc-1", Root: api, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Impact:  []inventory.EntityID{api},
		Members: members(crash),
		Cause: &rootcause.CauseRecord{Rule: "secret-missing-or-changed",
			Root: secret, Score: 0.9,
			Chain: []inventory.EntityID{secret, api},
			Proof: []rootcause.Proof{{Code: rootcause.ProofErrorsName,
				Count: count, Total: total, Weight: 0.8, Supports: true}}}}
}

// "The error mentions it" is only said next to the error it is about.
func TestErrorMentionsIsSaidNextToTheQuotedError(t *testing.T) {
	line := `secret "db-creds" not found`
	note := Writer{}.Write(announce(secretMentioned(1, 1, line)),
		at(6, 0)).Note

	assert.Contains(t, note, `It fails with "`+line+`". The error `+
		`mentions it.`)
}

func TestErrorMentionsIsDroppedWithoutAnErrorToQuote(t *testing.T) {
	note := Writer{}.Write(announce(secretMentioned(1, 1, "")),
		at(6, 0)).Note

	assert.NotContains(t, note, "mention")
}

// Of several errors only some name the cause: the quoted one is not
// shown as one of them unless it does.
func TestSomeErrorsMentionItNeedsAQuotedErrorThatDoes(t *testing.T) {
	other := Writer{}.Write(announce(secretMentioned(1, 3,
		"connection refused")), at(6, 0)).Note
	named := Writer{}.Write(announce(secretMentioned(1, 3,
		`secret "db-creds" not found`)), at(6, 0)).Note

	assert.NotContains(t, other, "mention")
	assert.Contains(t, named, "One of the three errors mentions it.")
}
