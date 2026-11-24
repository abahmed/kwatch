package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node that has just joined cannot give its first pods a network for
// a minute or two while its network plugin starts. The detectors hold
// those failures back while the node is young, so they rarely reach an
// incident; when they do (a node whose age the hold could not read
// yet), a pod network setup failure on a node younger than
// kube.YoungNodeWindow never pages. It still notifies, and pages as
// usual once the node is no longer new.

// levelYoungNetwork is the level code of an incident kept from paging
// by youngNetworkOnly.
const levelYoungNetwork = "young-node-network"

// sandboxReasons are the reasons of a pod that cannot get a network and
// of a node whose pods cannot.
var sandboxReasons = map[string]bool{
	"FailedCreatePodSandBox": true, "NetworkNotReady": true,
	reasons.NodeCNINotReady: true, reasons.NodePodIPExhausted: true,
	reasons.NodePodNetworkConflict: true,
}

// noteServing records, with the impact, what the page rules read:
// lost traffic, a user-facing workload with nothing ready, and whether
// only young nodes' network setup is critical.
func (p *Incident) noteServing(model inventory.Reader, now time.Time) {
	p.trafficLost = trafficLost(model, p)
	p.servingDown = servingDown(model, p)
	p.youngNetwork = youngNetworkOnly(model, p, now)
}

// youngNetworkOnly reports an incident whose critical members are all
// pod network setup failures on nodes younger than kube.YoungNodeWindow.
func youngNetworkOnly(
	model inventory.Reader, p *Incident, now time.Time,
) bool {
	if model == nil {
		return false
	}
	found := false
	for _, s := range p.Members {
		if s.Severity < detection.Critical {
			continue
		}
		if !sandboxReasons[s.Reason] || !onYoungNodes(model, s, now) {
			return false
		}
		found = true
	}
	return found
}

// onYoungNodes reports a finding of a node, or of pods, every one of
// which is younger than kube.YoungNodeWindow.
func onYoungNodes(
	model inventory.Reader, s detection.Finding, now time.Time,
) bool {
	nodes := []inventory.EntityID{s.Entity}
	if s.Entity.Kind != kube.KindNode {
		nodes = nil
		for _, pod := range failurePods(model, s.Entity) {
			nodes = append(nodes, model.Related(pod, inventory.RunsOn,
				inventory.Outgoing)...)
		}
	}
	for _, node := range nodes {
		if kube.NodeYoungWithin(model, node, now,
			kube.YoungNodeWindow) <= 0 {
			return false
		}
	}
	return len(nodes) > 0
}
