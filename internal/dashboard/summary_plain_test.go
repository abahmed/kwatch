package dashboard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestSummaryWhatOfANewVersionThatCannotStart(t *testing.T) {
	p := summaryIncident()
	p.Root.Name, p.Mode = "tracking", detection.ModeRolloutStuck
	p.PreviousServes = true
	p.Cause = &rootcause.CauseRecord{SharedError: "Unhandled " +
		"exception. System.InvalidOperationException: bad config"}
	d := toDetail(p)

	assert.Equal(t, "The new version of tracking crashes every time it "+
		"starts. The previous version is still serving users.",
		whatOf(&d, p))

	p.State = incident.Resolved
	assert.Equal(t, "The new version of tracking crashed every time it "+
		"started. The previous version kept serving users.",
		whatOf(&d, p))
}

// Each family has a plain sentence: no engine wording, no raw reason.
func TestSummaryWhatIsPlainForEveryFamily(t *testing.T) {
	cases := []struct {
		mode detection.Mode
		prev bool
		want string
	}{
		{detection.ModeRolloutStuck, true, "The new version of api " +
			"keeps failing to start. The previous version is still " +
			"serving users."},
		{detection.ModeRolloutStuck, false, "Deployment api in shop " +
			"has a new version that is not getting ready."},
		{detection.ModeCrashLoop, true, "The new version of api keeps " +
			"crashing. The previous version is still serving users."},
		{detection.ModeCrashLoop, false,
			"Deployment api in shop keeps crashing."},
		{detection.ModeImagePull, true, "The new version of api can't " +
			"pull its image. The previous version is still serving " +
			"users."},
		{detection.ModeImagePull, false,
			"Deployment api in shop can't pull its image."},
		{detection.ModeOOMKilled, true, "The new version of api keeps " +
			"running out of memory. The previous version is still " +
			"serving users."},
		{detection.ModeOOMKilled, false,
			"Deployment api in shop keeps running out of memory."},
		{detection.ModeUnschedulable, true, "The new version of api " +
			"can't be placed on a node. The previous version is still " +
			"serving users."},
		{detection.ModePending, false,
			"Deployment api in shop is stuck pending."},
		{detection.ModeNotReady, true, "The new version of api never " +
			"becomes ready. The previous version is still serving " +
			"users."},
		{detection.ModeMemoryPressure, false,
			"Deployment api in shop is low on memory."},
		{detection.ModeNodeCondition, false,
			"Deployment api in shop is not working properly."},
		{detection.ModeMissingSecret, false,
			"Deployment api in shop can't find a Secret it needs."},
		{detection.ModeMissingConfigMap, false,
			"Deployment api in shop can't find a ConfigMap it needs."},
		{detection.ModeJobFailed, false, "Deployment api in shop failed."},
		{detection.ModeCertExpired, false, "Deployment api in shop has " +
			"a certificate that has expired."},
		{detection.ModeScalingMaxedOut, false,
			"Deployment api in shop can't scale automatically."},
		{detection.ModeNoEndpoints, false, "Deployment api in shop has " +
			"no ready pods to send traffic to."},
		{detection.ModeBackendMissing, false, "Deployment api in shop " +
			"sends traffic to a Service that does not exist."},
	}
	for _, c := range cases {
		p := summaryIncident()
		p.Mode, p.PreviousServes = c.mode, c.prev
		d := toDetail(p)

		got := whatOf(&d, p)

		assert.Equal(t, c.want, got, string(c.mode))
		for _, bad := range []string{"stuck rollout", "explains",
			string(c.mode)} {
			assert.False(t, strings.Contains(got, bad),
				"%s says %q: %s", c.mode, bad, got)
		}
	}
}

// A failing probe on the new version is named, not the rollout.
func TestSummaryWhatOfANewVersionFailingItsProbe(t *testing.T) {
	p := summaryIncident()
	p.Mode, p.PreviousServes = detection.ModeRolloutStuck, true
	d := toDetail(p)
	d.Probes = []ProbeRow{{Probe: "readiness"}}

	assert.Equal(t, "The new version of api keeps failing its "+
		"readiness check. The previous version is still serving users.",
		whatOf(&d, p))
}
