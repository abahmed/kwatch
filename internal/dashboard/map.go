package dashboard

import (
	"slices"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// mapLevelOK is the level of an object no open issue touches.
const mapLevelOK = "ok"

// mapScope reports a namespace a picture may show: watched and not
// excluded. Cluster-scoped objects (namespace "") always pass.
func mapScope(scope Scope, namespace string) bool {
	if namespace == "" {
		return true
	}
	if slices.Contains(scope.Excluded, namespace) {
		return false
	}
	return len(scope.Watched) == 0 ||
		slices.Contains(scope.Watched, namespace)
}

// tierRank orders the levels a node can take; ok is the lowest.
func tierRank(level string) int {
	switch level {
	case incident.Page.String():
		return 3
	case incident.Notify.String():
		return 2
	case incident.Digest.String():
		return 1
	}
	return 0
}

// nodeID is a picture node's stable ID.
func nodeID(id inventory.EntityID) string { return id.String() }
