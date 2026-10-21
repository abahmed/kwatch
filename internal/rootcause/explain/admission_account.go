package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A controller that cannot create pods because the ServiceAccount they
// run as does not exist says so in its error. The account is the cause
// whether it was deleted a minute ago or was never made.

// missingAccount finds the account a refused create names, in
// lower-case text: serviceaccount "api" not found.
var missingAccount = regexp.MustCompile(`serviceaccount "([^"]+)" not ` +
	`found`)

// accountPath finds the namespace the same error names: "error looking
// up service account shop/api".
var accountPath = regexp.MustCompile(`service account ([a-z0-9.-]+)/` +
	`([a-z0-9.-]+)`)

// namedAccount is the ServiceAccount a failed create names. The error's
// own namespace wins; a bare name is one of the controller's namespace.
func (v *view) namedAccount(
	controller inventory.EntityID,
) (inventory.EntityID, bool) {
	text := strings.ToLower(v.errorText(controller))
	name := firstNameIn(text, missingAccount)
	if name == "" {
		return inventory.EntityID{}, false
	}
	namespace := controller.Namespace
	for _, m := range accountPath.FindAllStringSubmatch(text, -1) {
		if m[2] == name {
			namespace = m[1]
		}
	}
	return inventory.CoreID(kube.KindAccount, namespace, name), true
}

// accountAdmitHops lead from a controller whose creates were refused to
// the account its error says was not found. The link is the one the
// template's reference stores, so the hop is the same either way.
func (v *view) accountAdmitHops(controller inventory.EntityID) []hop {
	account, ok := v.namedAccount(controller)
	if !ok {
		return nil
	}
	return []hop{{link: LinkUses, to: account}}
}

// SignalNoAccount is a refused create that says its ServiceAccount was
// not found.
const SignalNoAccount Signal = "no-account-error"
