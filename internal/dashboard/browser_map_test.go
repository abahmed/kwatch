//go:build browser

package dashboard

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

// browserMapModel is a small namespace for the Map page to draw in the
// browser check: an Ingress, its Service, a Deployment with a pod, and
// a Service of another namespace the pod calls (mapShop).
func browserMapModel(t *testing.T) inventory.HistoryReader {
	tm, _ := mapShop(t)
	return tm.m
}
