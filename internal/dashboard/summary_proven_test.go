package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// An image proven not to be built for its node's CPU is the "Why?",
// as the title says it, with the release after it; never only "the
// common cause of 3 failing objects".
func TestCauseSentenceLeadsWithTheProvenCPU(t *testing.T) {
	p := summaryIncident()
	p.Cause = &rootcause.CauseRecord{
		Summary: "deployment api explains 3 failures", Score: 0.9}
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "shop", Name: "api-1/app"}
	p.Members = map[detection.Key]detection.Finding{
		{Entity: container, Reason: "CrashLoopBackOff"}: {
			Entity: container, Reason: "CrashLoopBackOff",
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceArchNode, Value: "arm64 node n1"},
				{Label: detection.EvidenceArchImage,
					Value: "registry.example.com/team/api:2.2"},
			}},
		{Entity: p.Root, Reason: "DeploymentUnavailable"}: {
			Entity: p.Root, Reason: "DeploymentUnavailable"},
	}
	d := toDetail(p)
	d.RootCause = p.Cause.Summary

	assert.Equal(t, "Its image api:2.2 is not built for the CPU of "+
		"arm64 node n1, so it fails with \"exec format error\".",
		whyText(&d, p).Text)

	d.Release = &ReleaseCause{Record: release.Record{
		At: p.Onset().Add(-time.Minute), Images: []release.Image{{
			Before: "registry.example.com/team/api:2.1",
			After:  "registry.example.com/team/api:2.2"}}}}
	assert.Equal(t, "Its image api:2.2 is not built for the CPU of "+
		"arm64 node n1, so it fails with \"exec format error\". It "+
		"started 1 minute after the release (image 2.1 → 2.2).",
		whyText(&d, p).Text)
}
