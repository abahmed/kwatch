package app

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config/crd"
)

// firstStartAPIBudget caps the startup API retries of a first start. No
// health listener exists yet, so the kubelet's startupProbe (300s in
// deploy/deploy.yaml) restarts a pod that is still retrying: the budget
// plus one attempt's timeout stays under it, and the start then fails
// with exit 1 as before. A rebuild retries until a stop, because the
// previous generation's listener keeps answering meanwhile.
const firstStartAPIBudget = 4 * time.Minute

// rebuildAuthWindow is how long a rebuild retries Unauthorized and
// Forbidden, which an auth webhook still down returns, before it fails
// as a first start would: a permission that is really gone then ends the
// process instead of retrying unseen.
const rebuildAuthWindow = 5 * time.Minute

// rebuildNotFoundWindow is how long a rebuild retries a NotFound for the
// KwatchConfig type before it accepts that the type is not installed.
// Right after an API outage the type can be missing for a moment.
const rebuildNotFoundWindow = 45 * time.Second

// apiRetry paces the startup API call of one generation. report, when
// set, names the outage on the previous generation's health listener. A
// rebuild also retries Unauthorized and Forbidden for rebuildAuthWindow
// and a NotFound for the KwatchConfig type for rebuildNotFoundWindow; a
// first start fails on the first two and takes a NotFound as no
// KwatchConfig at once.
func (g generationEnv) apiRetry(
	rebuilding bool, report func(reason string),
) *crd.StartupRetry {
	budget := firstStartAPIBudget
	var auth, notFound time.Duration
	if rebuilding {
		budget, auth, notFound = 0, rebuildAuthWindow,
			rebuildNotFoundWindow
	}
	return &crd.StartupRetry{
		Budget: budget, Now: g.now, Sleep: g.apiSleep,
		AuthWindow: auth, NotFoundWindow: notFound,
		OnRetry: func(attempt int, wait time.Duration, err error) {
			klog.InfoS("API server unreachable at startup; retrying",
				"component", "app", "attempt", attempt,
				"retryIn", wait, "error", err.Error())
			if report != nil {
				report("api_unreachable")
			}
		},
	}
}
