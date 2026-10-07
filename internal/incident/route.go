package incident

import "slices"

// AnnouncedRoute is how the first announcement of an incident was
// routed to providers: the namespaces and finding reasons it named and
// its severity ("critical", "warning" or "info"). It is recorded when
// the incident is announced, widened by every update that is sent (a
// later update can match a pager route the announcement did not), and
// persisted. The resolve is routed with it whole: by then the members
// are gone, so a route rebuilt from them would match no rule that asked
// for a reason, and a tier that fell since would change the severity
// the alert was opened with.
type AnnouncedRoute struct {
	Namespaces []string
	Reasons    []string
	Severity   string
	// Owners are the owners the incident had when it was told about;
	// a route that asks for an owner matches on them.
	Owners []string `json:",omitempty"`
}

// clone copies the route so a snapshot never shares its slices.
func (r *AnnouncedRoute) clone() *AnnouncedRoute {
	if r == nil {
		return nil
	}
	return &AnnouncedRoute{
		Namespaces: slices.Clone(r.Namespaces),
		Reasons:    slices.Clone(r.Reasons), Severity: r.Severity,
		Owners: slices.Clone(r.Owners),
	}
}

// widen adds what other names to r: the union of namespaces and
// reasons, and the higher severity.
func (r *AnnouncedRoute) widen(other *AnnouncedRoute) {
	r.Namespaces = unionSorted(r.Namespaces, other.Namespaces)
	r.Reasons = unionSorted(r.Reasons, other.Reasons)
	r.Owners = unionSorted(r.Owners, other.Owners)
	if severityRank(other.Severity) > severityRank(r.Severity) {
		r.Severity = other.Severity
	}
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	}
	return 0
}

func unionSorted(a, b []string) []string {
	set := map[string]bool{}
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		set[v] = true
	}
	return sortedKeys(set)
}

// routeOf summarises p for provider routing, the way a message is
// routed: the root's and the members' namespaces, the members' reasons,
// and the severity of the tier.
func routeOf(p *Incident) *AnnouncedRoute {
	namespaces, reasons := map[string]bool{}, map[string]bool{}
	if p.Root.Namespace != "" {
		namespaces[p.Root.Namespace] = true
	}
	for _, s := range p.Members {
		if s.Entity.Namespace != "" {
			namespaces[s.Entity.Namespace] = true
		}
		reasons[s.Reason] = true
	}
	return &AnnouncedRoute{
		Namespaces: sortedKeys(namespaces), Reasons: sortedKeys(reasons),
		Severity: severityOfTier(p.Tier), Owners: ownersOf(p.Owner),
	}
}

func severityOfTier(t Tier) string {
	switch t {
	case Page:
		return "critical"
	case Digest, Silent:
		return "info"
	}
	return "warning"
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
