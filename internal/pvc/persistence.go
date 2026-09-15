package pvc

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func volumeFacts(pvName string) model.Facts {
	return model.Facts{Volume: pvName}
}

func (p *PvcMonitor) report(obs *model.Observation) {
	if obs == nil || p.incidentSink == nil {
		return
	}
	p.incidentSink.Process(obs)
}

func (p *PvcMonitor) resolve(subject model.ObjectRef) {
	if p.incidentSink != nil {
		p.incidentSink.Resolve(subject, "")
	}
}

// persist saves a defensive state snapshot after producers have finished.
func (p *PvcMonitor) persist(ctx context.Context) {
	if p.state == nil {
		return
	}
	p.mu.RLock()
	usage := make(map[string]model.PVCSample, len(p.lastUsage))
	for key, value := range p.lastUsage {
		usage[key] = value
	}
	p.mu.RUnlock()
	if err := p.state.SavePvcUsage(ctx, usage); err != nil {
		klog.ErrorS(err, "pvc monitor: persist usage failed")
	}
}

func (p *PvcMonitor) restore(ctx context.Context) {
	if p.state == nil {
		return
	}
	seed := p.state.GetPvcUsage(ctx)
	p.mu.Lock()
	if seed != nil {
		p.lastUsage = cloneSamples(seed)
	}
	p.lastUsage = rekeyByClaim(p.lastUsage)
	var restore []*model.Observation
	for key, sample := range p.lastUsage {
		if !p.namespaceAllowedLocked(sample.Namespace) {
			delete(p.lastUsage, key)
			delete(p.notifiedPvc, key)
			continue
		}
		if sample.Pct < p.config.Threshold {
			continue
		}
		p.notifiedPvc[key] = true
		severity := model.SeverityNormal
		if sample.Pct >= p.config.CriticalThreshold {
			severity = model.SeverityHigh
		}
		restore = append(restore, observe.VolumeUsage(
			sample.Namespace, sample.Name, sample.PodName,
			constant.ReasonVolumeUsageHigh,
		).WithSeverity(severity).WithFacts(volumeFacts(sample.PVName)).WithHint(
			fmt.Sprintf("VolumeUsage(%.0f%%)", sample.Pct),
		))
	}
	p.mu.Unlock()
	sortObservations(restore)
	for _, obs := range restore {
		p.report(obs)
	}
}
