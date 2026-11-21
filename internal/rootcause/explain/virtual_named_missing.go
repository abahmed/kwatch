package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Kinds of the storage objects a storage error can name as missing.
const (
	kindCSIDriver       inventory.Kind = "csidriver"
	kindSnapshotContent inventory.Kind = "volumesnapshotcontent"
	kindVolumeSnapshot  inventory.Kind = "volumesnapshot"
	snapshotGroup                      = "snapshot.storage.k8s.io"
)

// namedMissing says that an error of one kind of entity names an object
// of another kind, and where. The object is the cause when it does not
// exist.
type namedMissing struct {
	// from is the kind of entity whose error names the object.
	from inventory.Kind
	// pattern finds the name in lower-case error text: group 1.
	pattern *regexp.Regexp
	// group, kind and namespace-less name build the missing object.
	group string
	kind  inventory.Kind
}

// namedMissingTable lists the errors that name a missing storage
// object. The kubelet waits for a volume attachment that a driver
// without a CSIDriver object never gets; the snapshot controller
// reports the content a snapshot names.
var namedMissingTable = []namedMissing{
	{from: kube.KindPod, kind: kindCSIDriver,
		pattern: regexp.MustCompile(
			`volume attachment for driver ([a-z0-9][a-z0-9._-]*)`)},
	{from: kindVolumeSnapshot, group: snapshotGroup,
		kind: kindSnapshotContent,
		pattern: regexp.MustCompile(
			`volumesnapshotcontent "([^"]+)" is missing`)},
}

// namedMissingRows say the object an error names, when it does not
// exist, explains the object whose error names it.
var namedMissingRows = []Row{
	{
		Name:   "used-missing",
		Cause:  Side{Kind: kindCSIDriver, Modes: missingOnly},
		Link:   LinkUses,
		Effect: podSide(modeFailedMount, modeFailedAttach),
		Prior:  0.7,
	},
	{
		Name: "used-missing",
		Cause: Side{Group: snapshotGroup, Kind: kindSnapshotContent,
			Modes: missingOnly},
		Link:   LinkUses,
		Effect: Side{Group: snapshotGroup, Kind: kindVolumeSnapshot},
		Prior:  0.8,
	},
}

var missingOnly = []detection.Mode{ModeMissing}

// namedMissingHops lead from an entity whose error names a storage
// object that does not exist to that object.
func (v *view) namedMissingHops(id inventory.EntityID) []hop {
	var out []inventory.EntityID
	for _, n := range namedMissingTable {
		if id.Kind != n.from || len(v.s.Findings[id]) == 0 ||
			!v.s.synced(n.kind) {
			continue
		}
		name := firstNameIn(strings.ToLower(v.text(id)), n.pattern)
		if name == "" {
			continue
		}
		target := inventory.NewEntityID(n.group, n.kind, "", name)
		if !v.s.Model.Exists(target) {
			out = append(out, target)
		}
	}
	return hopsTo(LinkUses, out...)
}
