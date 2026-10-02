package incident

import (
	"sort"
	"strings"
)

// severityTiers maps the configured severity vocabulary to a tier.
var severityTiers = map[string]Tier{
	"critical": Page,
	"high":     Notify,
	"medium":   Notify,
	"warning":  Notify,
	"low":      Digest,
	"info":     Digest,
	"normal":   Digest,
}

// severityTier converts a configured severity to a tier.
func severityTier(severity string) (Tier, bool) {
	t, ok := severityTiers[strings.ToLower(strings.TrimSpace(severity))]
	return t, ok
}

// overrides holds user severity overrides compiled to tiers, keyed by
// lower-cased reason or owner kind. Unknown severities are dropped.
type overrides struct {
	byReason map[string]Tier
	byKind   map[string]Tier
}

func newOverrides(byReason, byKind map[string]string) overrides {
	return overrides{
		byReason: compileOverrides(byReason),
		byKind:   compileOverrides(byKind),
	}
}

// compileOverrides lower-cases the keys. Keys that differ only in case are
// rejected by config validation; here the lexicographically first one wins
// so the result never depends on map iteration order.
func compileOverrides(in map[string]string) map[string]Tier {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]Tier, len(in))
	for _, key := range keys {
		t, ok := severityTier(in[key])
		if !ok {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(key))
		if _, taken := out[lower]; !taken {
			out[lower] = t
		}
	}
	return out
}

// apply returns the tier after overrides. Reason overrides win over owner
// kind overrides; among matches the loudest tier wins. An incident without
// members is never overridden.
func (o overrides) apply(p *Incident, base Tier) Tier {
	if len(p.Members) == 0 {
		return base
	}
	loudest, found := Silent, false
	for _, s := range p.Members {
		if t, ok := o.byReason[strings.ToLower(s.Reason)]; ok {
			loudest, found = max(loudest, t), true
		}
	}
	if found {
		return loudest
	}
	kinds := []string{string(p.Root.Kind)}
	for _, id := range p.Impact {
		kinds = append(kinds, string(id.Kind))
	}
	for _, kind := range kinds {
		if t, ok := o.byKind[strings.ToLower(kind)]; ok {
			loudest, found = max(loudest, t), true
		}
	}
	if found {
		return loudest
	}
	return base
}
