package explain

import (
	"slices"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeTiming is a probe's timing as the field names write it:
// "initialDelaySeconds 10, periodSeconds 10, failureThreshold 3". The
// liveness probe also has the older liveness attributes, which fill in
// for a container recorded before probe settings were kept. It is ""
// when the container records no timing for the probe.
func probeTiming(container inventory.Entity, probe string) string {
	fields := []struct{ name, setting, old string }{
		{"initialDelaySeconds", "delay", kube.AttrLivenessDelay},
		{"periodSeconds", "period", kube.AttrLivenessPeriod},
		{"failureThreshold", "failures", kube.AttrLivenessFailures},
	}
	var parts []string
	for _, field := range fields {
		value, ok := attrNumber(container,
			kube.ProbeAttr(probe, field.setting))
		if !ok && probe == "liveness" {
			value, ok = attrNumber(container, field.old)
		}
		if !ok {
			return ""
		}
		parts = append(parts, field.name+" "+
			strconv.FormatFloat(value, 'f', -1, 64))
	}
	return strings.Join(parts, ", ")
}

// livenessPlace is the liveness probe of a container killed before it
// is ready: how long it waits and how many failures it allows.
func (s Snapshot) livenessPlace(c Cause) *detection.Place {
	for _, container := range s.containersOf(c) {
		if timing := probeTiming(container, "liveness"); timing != "" {
			return s.containerPlace(container, "livenessProbe", timing)
		}
	}
	return nil
}

// startupBudgetPlace is the probe whose budget is shorter than the
// start: the startup probe when there is one, else the readiness probe,
// the same one the budget attribute adds up.
func (s Snapshot) startupBudgetPlace(c Cause) *detection.Place {
	for _, container := range s.containersOf(c) {
		if _, ok := attrNumber(container, kube.AttrProbeBudget); !ok {
			continue
		}
		for _, probe := range []string{"startup", "readiness"} {
			if timing := probeTiming(container, probe); timing != "" {
				return s.containerPlace(container, probe+"Probe", timing)
			}
		}
	}
	return nil
}

// probePortPlace is the probe aimed at a port the container does not
// declare.
func (s Snapshot) probePortPlace(c Cause) *detection.Place {
	for _, container := range s.containersOf(c) {
		if !probeMissesPorts(container) {
			continue
		}
		declared := strings.Split(
			attrText(container, kube.AttrContainerPorts), ",")
		for _, probe := range kube.ProbeKinds {
			port := attrText(container, kube.ProbeAttr(probe, "port"))
			handler := attrText(container, kube.ProbeAttr(probe, "handler"))
			if port == "" || handler == "" || slices.Contains(declared, port) {
				continue
			}
			return s.containerPlace(container,
				probe+"Probe."+handler+".port", port)
		}
	}
	return nil
}
