package incident

import "strconv"

// While an incident flaps, its updates are held: the failure comes and
// goes, and each swing would be a message. Two things still break the
// silence. The failure grows, because a flapping outage that gets worse
// is news; and a page that stays open is reminded of, as any page is.

// growthKey names how big the failure has been: the tier, the impact
// and the failing pods, each at its peak, so it only ever grows. The
// pods use the same buckets as the roll-up fingerprint: one or two more
// are not news, a tripling is.
func growthKey(p *Incident) string {
	return strconv.Itoa(int(p.Tier)) + "|" +
		impactBucket(max(p.impactPeak, impactSize(p))) + "|" +
		podBucket(max(p.Delivery.PodPeak(), failingPods(p)))
}

// flapGrew reports whether a flapping incident's failure grew since its
// last message. An incident with no remembered size, such as one restored
// from the state file, adopts its current size and says nothing.
func flapGrew(p *Incident) bool {
	p.Delivery.RaisePodPeak(failingPods(p))
	now := growthKey(p)
	if p.Delivery.SentGrowth() == "" {
		p.Delivery.AdoptGrowth(now)
		return false
	}
	return now != p.Delivery.SentGrowth()
}
