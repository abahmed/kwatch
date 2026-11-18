package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// SummaryGrouped says why an issue holds several workloads or objects
// (incident.GroupingOf): Text is one sentence, Basis what it rests on
// (node, place, namespace, error, image, release, dependency, chain,
// cause or leftover), Groups the members by the workload or object
// they belong to, the largest first; More counts the groups left out.
type SummaryGrouped struct {
	Text   string         `json:"text"`
	Basis  string         `json:"basis"`
	Groups []SummaryGroup `json:"groups,omitempty"`
	More   int            `json:"more,omitempty"`
}

// SummaryGroup is one workload, or one object without an owner, with
// its failing members; More counts the members left out.
type SummaryGroup struct {
	Owner   SummaryObject   `json:"owner"`
	Members []SummaryObject `json:"members"`
	More    int             `json:"more,omitempty"`
}

// SummaryObject names one object.
type SummaryObject struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// SummaryUsers says whether users reach what fails
// (incident.UsersOf). State is affected, served, no-route or
// no-service; Route, Service, Ready and Total are the route and the
// Service it names, Callers the other workloads that call it.
type SummaryUsers struct {
	Text    string   `json:"text"`
	State   string   `json:"state"`
	Route   string   `json:"route,omitempty"`
	Service string   `json:"service,omitempty"`
	Ready   int      `json:"ready"`
	Total   int      `json:"total"`
	Callers []string `json:"callers,omitempty"`
}

// summaryShape is what the live model says about an issue's shape.
type summaryShape struct {
	grouping *incident.Grouping
	split    *incident.ReplicaSplit
	users    *incident.Users
}

// shapeOf reads p's shape from the model. The replica split and the
// users speak of now, so a resolved issue has neither.
func shapeOf(model inventory.Reader, p incident.Incident) *summaryShape {
	out := &summaryShape{grouping: incident.GroupingOf(model, p)}
	if p.State != incident.Resolved {
		out.split = incident.ReplicaSplitOf(model, p)
		out.users = incident.UsersOf(model, p)
	}
	return out
}

// apply puts the shape in the summary, redacted and bounded. A nil
// shape adds nothing.
func (s *summaryShape) apply(out *IncidentSummary) {
	if s == nil {
		return
	}
	if g := s.grouping; g != nil {
		out.Grouped = groupedOf(g)
	}
	if s.split != nil {
		for _, line := range s.split.Lines {
			text, _ := shortText(redact.Evidence(line), maxSummaryText)
			out.Replicas = append(out.Replicas, text)
		}
	}
	if u := s.users; u != nil {
		text, _ := shortText(redact.Evidence(u.Text), maxSummaryText)
		out.Users = &SummaryUsers{Text: text, State: u.State,
			Route: redact.Evidence(u.Route), Service: u.Service,
			Ready: u.Ready, Total: u.Total, Callers: u.Callers}
	}
}

// groupedOf is the grouping as the summary carries it.
func groupedOf(g *incident.Grouping) *SummaryGrouped {
	text, _ := shortText(redact.Evidence(g.Text), maxSummaryText)
	out := &SummaryGrouped{Text: text, Basis: g.Basis, More: g.More}
	for _, group := range g.Groups {
		row := SummaryGroup{Owner: objectOf(group.Owner), More: group.More}
		for _, m := range group.Members {
			row.Members = append(row.Members, objectOf(m))
		}
		out.Groups = append(out.Groups, row)
	}
	return out
}

func objectOf(id inventory.EntityID) SummaryObject {
	return SummaryObject{Kind: string(id.Kind), Namespace: id.Namespace,
		Name: id.Name}
}
