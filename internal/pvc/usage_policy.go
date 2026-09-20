package pvc

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (p *PvcMonitor) effectiveClear() float64 {
	clear := p.config.ClearThreshold
	if clear <= 0 || clear > p.config.Threshold {
		clear = p.config.Threshold
	}
	return clear
}

func (p *PvcMonitor) cacheSample(
	now time.Time, usage *PvcUsage, clear float64,
) {
	if usage.UsagePercentage >= clear {
		p.lastUsage[usage.key()] = model.PVCSample{
			Pct: usage.UsagePercentage, Namespace: usage.Namespace,
			Name: usage.Name, PodName: usage.PodName, Seen: now,
			PVName: usage.PVName,
		}
	} else {
		delete(p.lastUsage, usage.key())
	}
}

func (p *PvcMonitor) signalIfOver(
	usage *PvcUsage,
	isSweep bool,
	currentNotified map[string]bool,
	observations *[]*model.Observation,
) {
	key := usage.key()
	wasNotified := p.notifiedPvc[key]
	currentNotified[key] = true
	if p.firstScan {
		return
	}
	if isSweep || !wasNotified {
		severity := model.SeverityNormal
		if usage.UsagePercentage >= p.config.CriticalThreshold {
			severity = model.SeverityHigh
		}
		*observations = append(*observations, observe.VolumeUsage(
			usage.Namespace, usage.Name, usage.PodName,
			constant.ReasonVolumeUsageHigh,
		).WithSeverity(severity).WithFacts(volumeFacts(usage.PVName)).WithHint(
			fmt.Sprintf("VolumeUsage(%.0f%%)", usage.UsagePercentage),
		))
	}
}

func (p *PvcMonitor) resolveStale(
	seen, bound, currentNotified map[string]bool,
	clear float64,
	resolves *[]model.ObjectRef,
) {
	for key := range p.notifiedPvc {
		if currentNotified[key] {
			continue
		}
		switch {
		case seen[key]:
			*resolves = append(*resolves, claimRef(key))
			delete(p.lastUsage, key)
		case !bound[key]:
			*resolves = append(*resolves, claimRef(key))
			delete(p.lastUsage, key)
		default:
			if sample, ok := p.lastUsage[key]; ok && sample.Pct >= clear {
				currentNotified[key] = true
			} else {
				*resolves = append(*resolves, claimRef(key))
				delete(p.lastUsage, key)
			}
		}
	}
}

func (p *PvcMonitor) apply(
	pvcUsages []*PvcUsage,
	pvByPVC map[string]string,
	incomplete bool,
	isSweep bool,
) {
	p.mu.Lock()
	now := p.now()
	clear := p.effectiveClear()
	currentNotified := make(map[string]bool, len(pvcUsages))
	seen := make(map[string]bool, len(pvcUsages))
	var observations []*model.Observation
	var resolves []model.ObjectRef
	for _, pvc := range pvcUsages {
		seen[pvc.key()] = true
		p.cacheSample(now, pvc, clear)
		if pvc.UsagePercentage >= p.config.Threshold {
			p.signalIfOver(pvc, isSweep, currentNotified, &observations)
		} else if p.notifiedPvc[pvc.key()] &&
			pvc.UsagePercentage >= clear {
			currentNotified[pvc.key()] = true
		}
	}
	if isSweep && p.firstScan {
		p.firstScan = false
	}
	bound := make(map[string]bool, len(pvByPVC))
	for key := range pvByPVC {
		bound[key] = true
	}
	if !incomplete {
		p.resolveStale(seen, bound, currentNotified, clear, &resolves)
	}
	if !incomplete {
		p.notifiedPvc = currentNotified
	} else {
		for key := range currentNotified {
			p.notifiedPvc[key] = true
		}
	}
	p.mu.Unlock()

	for _, observation := range observations {
		p.report(observation)
	}
	for _, reference := range resolves {
		p.resolve(reference)
	}
}

func claimRef(key string) model.ObjectRef {
	namespace, name, _ := strings.Cut(key, "/")
	return model.NewObjectRef("pvc", namespace, name)
}
