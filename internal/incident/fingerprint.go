package incident

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// fingerprint hashes what a reader would notice: tier, root, the cause's
// identity, the root's own conditions and the size of the impact. It is
// stored as Incident.Digest; an unchanged fingerprint is never re-sent.
// Symptoms of affected entities count only through the impact size,
// at its peak: only growth is news.
// Free text such as summaries carries live counters, percentages and
// estimates, so it never enters the fingerprint.
func fingerprint(p *Incident) string {
	reasons := make([]string, 0, len(p.Members)+len(p.rootReasons))
	for key, s := range p.Members {
		if key.Entity == p.Root && !s.Symptom {
			reasons = append(reasons, key.Reason)
		}
	}
	for reason := range p.rootReasons {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	parts := []string{
		strconv.Itoa(int(p.Tier)), p.Root.String(), causeIdentity(p),
		strings.Join(uniq(reasons), ","),
		impactBucket(max(p.impactPeak, impactSize(p))),
		strconv.Itoa(int(p.State)),
	}
	// Appended only when present so fingerprints of incidents without gaps
	// stay what earlier versions persisted.
	if len(p.Unverified) > 0 {
		parts = append(parts, strings.Join(p.Unverified, ","))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:hashBytes])
}

// causeIdentity names the cause by rule, blamed entity, the reasons of the
// root's own findings and the blamed change.
//
// A cause that blames the incident's own root and no change tells the
// reader nothing the fingerprint does not already hold, except root
// findings that are not members: the root and its member reasons count on
// their own. Only those extra findings name it, so a cause appearing for
// an incident that already stated its root is not news.
func causeIdentity(p *Incident) string {
	c := p.Cause
	if c == nil {
		return ""
	}
	change := ""
	if c.Change != nil {
		change = c.Change.Entity.String() + "@" + c.Change.Revision +
			"@" + c.Change.At.UTC().Format(time.RFC3339)
	}
	if c.Root == p.Root && change == "" {
		return strings.Join(rootFindingReasons(c, p.Members), ",")
	}
	return strings.Join([]string{
		c.Rule, c.Root.String(),
		strings.Join(rootFindingReasons(c, nil), ","), change,
	}, ";")
}

// rootFindingReasons lists the cause's root findings as sorted, unique
// "entity=reason" pairs, leaving out those that are members.
func rootFindingReasons(
	c *rootcause.CauseRecord, members map[detection.Key]detection.Finding,
) []string {
	reasons := make([]string, 0, len(c.RootFindings))
	for _, s := range c.RootFindings {
		if _, member := members[s.Key()]; member {
			continue
		}
		reasons = append(reasons, s.Entity.String()+"="+s.Reason)
	}
	sort.Strings(reasons)
	return uniq(reasons)
}

// rememberRootReasons adds the root's current own reasons to the set the
// fingerprint reads. A reason that clears while the incident stays open
// is not news; the incident's recovery is.
func (p *Incident) rememberRootReasons() {
	for key, s := range p.Members {
		if key.Entity != p.Root || s.Symptom {
			continue
		}
		if p.rootReasons == nil {
			p.rootReasons = map[string]struct{}{}
		}
		p.rootReasons[key.Reason] = struct{}{}
	}
}

// rootReasonList returns the remembered root reasons, sorted, for the
// persisted record.
func (p *Incident) rootReasonList() []string {
	out := make([]string, 0, len(p.rootReasons))
	for reason := range p.rootReasons {
		out = append(out, reason)
	}
	sort.Strings(out)
	return out
}

// impactSize counts affected workloads, Services and Ingresses. Pods and
// containers are excluded: replicas failing one by one are not news.
func impactSize(p *Incident) int {
	n := 0
	for _, id := range p.Impact {
		if id.Kind != kube.KindPod && id.Kind != kube.KindContainer {
			n++
		}
	}
	return n
}

// impactBucket changes only when impact crosses 1 → N or roughly doubles.
func impactBucket(n int) string {
	switch {
	case n <= 1:
		return "1"
	case n <= 3:
		return "2-3"
	case n <= 7:
		return "4-7"
	case n <= 15:
		return "8-15"
	default:
		return "16+"
	}
}
