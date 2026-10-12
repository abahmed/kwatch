package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func init() {
	scorers = append(scorers, scorer{"error-class", scoreErrorClass})
}

// errorClass is a family of failure messages and the kinds of object
// that can produce it. The table is data: a class says what its words
// fit, never which workload failed.
type errorClass struct {
	name string
	// markers are lower-case words of the messages in the class.
	markers []string
	// fits are the candidate kinds the class points at.
	fits []inventory.Kind
	// misfits are the kinds the class argues against.
	misfits []inventory.Kind
	// addressKeys lets a misfit kind off when a changed key names an
	// address: a changed DB_HOST can explain a timeout.
	addressKeys bool
}

var (
	configKinds = []inventory.Kind{kube.KindConfigMap, kube.KindSecret}
	// codeKinds are the workloads a new image or template changes.
	codeKinds = []inventory.Kind{kube.KindDeployment,
		kube.KindStatefulSet, kube.KindDaemonSet}
	// networkKinds are the objects that decide whether a call arrives.
	networkKinds = []inventory.Kind{kube.KindNetworkPolicy,
		kube.KindService, kube.KindNode, kube.KindExternalEndpoint}
)

// errorClasses are tried in order: the first class that names half of
// the failures decides.
var errorClasses = []errorClass{
	{name: "a configuration value the program cannot read",
		markers: []string{"parsing ", "invalid syntax", "unmarshal",
			"strconv.", "invalid character", "failed to parse",
			"cannot parse", "yaml:"},
		fits: configKinds},
	{name: "a defect in the program's own code",
		markers: []string{"nil pointer dereference", "index out of range",
			"slice bounds out of range", "runtime error",
			"segmentation fault"},
		fits: codeKinds},
	{name: "a connection that does not arrive",
		markers: []string{"i/o timeout", "connection refused",
			"no route to host", "connection timed out",
			"network is unreachable", "dial tcp"},
		fits: networkKinds, misfits: configKinds, addressKeys: true},
}

// addressKeyMarkers mark a config key that holds where to connect.
var addressKeyMarkers = []string{"host", "url", "uri", "addr", "port",
	"dsn", "endpoint", "server"}

// scoreErrorClass reads what the failures' errors say they are and asks
// whether the candidate is the kind of object that causes that.
func scoreErrorClass(v *view, c *candidate) outcome {
	effects := c.others()
	if len(effects) == 0 || c.isSelf() {
		return outcome{}
	}
	for _, class := range errorClasses {
		named := 0
		for _, effect := range effects {
			if hasMarker(v.errorText(effect), class.markers) {
				named++
			}
		}
		if named*2 < len(effects) {
			continue
		}
		return v.classOutcome(class, c)
	}
	return outcome{}
}

func (v *view) classOutcome(class errorClass, c *candidate) outcome {
	switch {
	case slices.Contains(class.fits, c.id.Kind):
		return outcome{weight: ErrorClassWeight,
			code: rootcause.ProofErrorFits,
			text: "the errors describe " + class.name +
				", which fits a " + string(c.id.Kind)}
	case slices.Contains(class.misfits, c.id.Kind) &&
		!(class.addressKeys && v.changesAddress(c.id)):
		return outcome{weight: -ErrorClassPenalty,
			code: rootcause.ProofErrorMisfits,
			text: "the errors describe " + class.name +
				", which a " + string(c.id.Kind) + " does not cause"}
	}
	return outcome{}
}

// changesAddress reports a changed key that holds an address.
func (v *view) changesAddress(id inventory.EntityID) bool {
	for _, key := range v.changedKeys(id) {
		if hasMarker(key, addressKeyMarkers) {
			return true
		}
	}
	return false
}

func hasMarker(text string, markers []string) bool {
	text = strings.ToLower(text)
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
