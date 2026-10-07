package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// loginIncident is a registry that refuses the logins of the failing
// pods.
func loginIncident(logins []rootcause.PullLogin, refused string,
) incident.Incident {
	registry := inventory.CoreID(kube.KindRegistry, "", "registry.example.com")
	api := inventory.CoreID(kube.KindDeployment, "ci", "api")
	ct := inventory.CoreID(kube.KindContainer, "ci", "api-4d/app")
	return incident.Incident{
		ID: "inc-9", Root: registry, Tier: incident.Notify,
		State: incident.Open, Opened: at(3, 0), Revision: 1,
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.ImagePullBackOff, Severity: detection.Critical,
			Since: at(3, 0), Summary: "Image pull is backing off"}),
		Cause: &rootcause.CauseRecord{Root: registry, Rule: "registry-refuses",
			Mode: "Auth", Score: 0.8,
			Chain:  []inventory.EntityID{registry, api, ct},
			Logins: logins, Refused: refused},
	}
}

func TestPullLoginNamesTheSecretItsTypeAndAge(t *testing.T) {
	now := at(10, 0)
	p := loginIncident([]rootcause.PullLogin{{
		Namespace: "ci", Secret: "regcred", State: rootcause.LoginFound,
		Type:    "kubernetes.io/dockerconfigjson",
		Changed: now.Add(-92 * 24 * time.Hour)}},
		"unauthorized: authentication required")

	text := notification.Text(Writer{}.Write(announce(p), now))

	assert.Contains(t, text, "It pulls with Secret regcred in ci "+
		"(kubernetes.io/dockerconfigjson, last changed 92 days ago), "+
		"and the registry answers "+
		"\"unauthorized: authentication required\".")
}

func TestPullLoginSaysWhenThereIsNoneOrItIsMissing(t *testing.T) {
	none := notification.Text(Writer{}.Write(announce(loginIncident(
		[]rootcause.PullLogin{{Namespace: "ci",
			State: rootcause.LoginNone}}, "")), at(10, 0)))
	assert.Contains(t, none, "Its pods name no pull secret, so the "+
		"registry sees anonymous pulls.")

	missing := notification.Text(Writer{}.Write(announce(loginIncident(
		[]rootcause.PullLogin{
			{Namespace: "ci", Secret: "backup", State: rootcause.LoginFound,
				Type: "kubernetes.io/dockercfg"},
			{Namespace: "ci", Secret: "regcred",
				State: rootcause.LoginMissing}}, "")), at(10, 0)))
	assert.Contains(t, missing, "It pulls with Secret backup in ci "+
		"(kubernetes.io/dockercfg) and Secret regcred in ci, which does "+
		"not exist.")
}

func TestPullLoginCallsOutASecretThatIsNotALogin(t *testing.T) {
	p := loginIncident([]rootcause.PullLogin{{Namespace: "ci",
		Secret: "regcred", State: rootcause.LoginFound, Type: "Opaque",
		Changed: at(10, 0).Add(-3 * time.Hour)}}, "")

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.Contains(t, text, "Secret regcred in ci (type Opaque, not a "+
		"registry login, last changed")
}

func TestOtherCausesSayNothingOfLogins(t *testing.T) {
	p := loginIncident(nil, "")
	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))
	assert.NotContains(t, text, "pulls with")
}
