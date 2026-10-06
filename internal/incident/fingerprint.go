package incident

import (
	"crypto/sha256"
	"encoding/hex"
	"math/bits"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Compatibility contract: the fingerprint is persisted as Incident.Digest
// and compared after a restart. Changing the parts of an existing
// incident (their order, their text, or adding one that is always
// present) makes every restored incident look changed and sends an
// update about nothing. So a new part is appended only under a condition
// that is false for incidents that never had it, and adoptFingerprints
// covers the rest.

// fingerprint hashes what a reader would notice: tier, root, the cause's
// identity, the root's own conditions and the size of the impact. It is
// stored as Incident.Digest; an unchanged fingerprint is never re-sent.
// Symptoms of affected entities count only through the impact size,
// at its peak: only growth is news.
// Free text such as summaries carries live counters, percentages and
// estimates, so it never enters the fingerprint.
func fingerprint(p *Incident) string {
	names := make([]string, 0, len(p.Members)+len(p.rootReasons))
	for key, s := range p.Members {
		if key.Entity == p.Root && !s.Symptom && !s.Advisory &&
			!restated(p, key.Reason) {
			names = append(names, key.Reason)
		}
	}
	for reason := range p.rootReasons {
		if !restated(p, reason) {
			names = append(names, reason)
		}
	}
	sort.Strings(names)
	parts := []string{
		strconv.Itoa(int(p.Tier)), p.Root.String(), causeIdentity(p),
		strings.Join(uniq(names), ","),
		impactBucket(max(p.impactPeak, impactSize(p))),
		strconv.Itoa(int(p.State)),
	}
	// Appended only when present so fingerprints of incidents without gaps
	// stay what earlier versions persisted.
	if len(p.Unverified) > 0 {
		parts = append(parts, strings.Join(p.Unverified, ","))
	}
	if p.stagePeak >= stageCrashLoop {
		// A member crash-looping is news once, whenever it begins.
		parts = append(parts, "crash-loop")
	}
	if p.Delivery.RolledUp() {
		// A roll-up named this incident once; more pods failing is
		// the news no other message would carry.
		parts = append(parts, "pods:"+podBucket(p.Delivery.PodPeak()))
	}
	if p.Attempt != nil {
		// Each fix attempt, and the word that it still fails, is news.
		parts = append(parts, "attempt:"+p.Attempt.Revision+"@"+
			p.Attempt.At.UTC().Format(time.RFC3339)+
			strconv.FormatBool(p.attemptLate))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:hashBytes])
}

// restated reports a reason that only repeats, at a later stage, what
// p already says. A workload whose Service has no endpoints is not
// serving, and people were told so; learning that its pods run but never
// become ready adds a stage to that story, not a new problem.
//
// A workload that is merely unavailable is different: "never became
// healthy" after minutes of waiting is a milestone of its own, so only
// the Service's word restates it.
func restated(p *Incident, reason string) bool {
	if reason != reasons.WorkloadNeverReady {
		return false
	}
	for key, s := range p.Members {
		if !s.Advisory && key.Reason == reasons.ServiceNoEndpoints {
			return true
		}
	}
	return false
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
	if p.causeChain && change == "" {
		// The cause flips between a pod, a Service and the workload
		// for the same failures. Only a reason never seen before in
		// this incident is news, whichever object carries it.
		return "chain:" + strings.Join(seenReasons(p.seenCauseFindings(c)),
			",")
	}
	return strings.Join([]string{
		c.Rule, c.Root.String(),
		strings.Join(p.seenCauseFindings(c), ","), change,
	}, ";")
}

// seenCauseFindings lists the cause's root findings and those it had
// earlier in this incident. A finding that ages out, such as a window of
// events, leaves the cause only because time passed; it is not news.
func (p *Incident) seenCauseFindings(c *rootcause.CauseRecord) []string {
	out := rootFindingReasons(c, nil)
	for pair := range p.causeFindings {
		out = append(out, pair)
	}
	sort.Strings(out)
	return uniq(out)
}

// seenReasons keeps the reason of each "entity=reason" pair, sorted and
// without repeats.
func seenReasons(pairs []string) []string {
	out := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, pair[strings.LastIndexByte(pair, '=')+1:])
	}
	sort.Strings(out)
	return uniq(out)
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

// maxCauseFindings bounds the root findings an open incident remembers.
// A chronic incident whose cause is a crash loop of replaced pods would
// otherwise add one entry for every pod it ever had.
const maxCauseFindings = 256

// rememberCauseFinding adds one "entity=reason" pair, unless the set is
// full and the pair is new.
func (p *Incident) rememberCauseFinding(pair string) {
	if p.causeFindings == nil {
		p.causeFindings = map[string]struct{}{}
	}
	if _, known := p.causeFindings[pair]; !known &&
		len(p.causeFindings) >= maxCauseFindings {
		return
	}
	p.causeFindings[pair] = struct{}{}
}

// rememberRootReasons adds the root's current own reasons to the set the
// fingerprint reads. A reason that clears while the incident stays open
// is not news; the incident's recovery is.
func (p *Incident) rememberRootReasons() {
	if p.Cause != nil {
		for _, pair := range rootFindingReasons(p.Cause, nil) {
			p.rememberCauseFinding(pair)
		}
	}
	for key, s := range p.Members {
		if key.Entity != p.Root || s.Symptom || s.Advisory {
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

// podBucket groups pod counts so that one or two more pods of a crash
// loop are not news but a tripling is: 0-2 pods, 3-5, 6-11, 12-23 and so
// on.
func podBucket(pods int) string {
	return strconv.Itoa(bits.Len(uint(pods / 3)))
}
