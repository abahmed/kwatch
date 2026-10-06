package storage

import (
	"time"

	"k8s.io/klog/v2"
)

// SlowStep is how long a startup step may take before it is logged on
// its own. Opening the state file can read all of it, so a slow volume
// shows here instead of as a silent gap after the Lease is acquired.
const SlowStep = 5 * time.Second

// startupNow is the clock of the startup timings. Tests replace it.
var startupNow = time.Now

// stepTimer measures one startup step.
type stepTimer struct {
	name    string
	started time.Time
}

func startStep(name string) stepTimer {
	return stepTimer{name: name, started: startupNow()}
}

// end returns how long the step took and logs it when it was slow.
func (s stepTimer) end() time.Duration {
	took := startupNow().Sub(s.started)
	LogIfSlow(s.name, took)
	return took
}

// LogIfSlow logs a startup step that took SlowStep or longer. The
// application uses it for the steps it times itself.
func LogIfSlow(step string, took time.Duration) {
	if took < SlowStep {
		return
	}
	klog.InfoS("state startup step was slow", "component", "state",
		"operation", "startup", "step", step,
		"durationMs", took.Milliseconds())
}
