package incident

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Radius is how far an incident reaches: the Services, Ingresses and
// workloads in its impact, and its failing pods. It only counts what
// the impact and the members already hold; nothing decides an alert
// from it.
type Radius struct {
	Services  int `json:",omitempty"`
	Ingresses int `json:",omitempty"`
	// Workloads counts the workloads in the impact other than Of.
	Workloads int `json:",omitempty"`
	Pods      int `json:",omitempty"`
	// Of is the workload every failing pod belongs to, and Replicas
	// the replicas it wants; zero when the pods belong to several
	// workloads or the count is unknown.
	Of       inventory.EntityID `json:",omitzero"`
	Replicas int                `json:",omitempty"`
}

// size is how many objects the radius counts.
func (r Radius) size() int {
	return r.Services + r.Ingresses + r.Workloads + r.Pods
}

// radiusOf counts what p reaches now.
func radiusOf(model inventory.Reader, p *Incident) Radius {
	var r Radius
	owners := map[inventory.EntityID]bool{}
	pods := map[inventory.EntityID]bool{}
	for key, f := range p.Members {
		if f.Advisory {
			continue
		}
		if pod, ok := rootcause.PodOf(model, key.Entity); ok {
			pods[pod] = true
			owners[rootcause.TopOwner(model, pod)] = true
		}
	}
	r.Pods = len(pods)
	if len(owners) == 1 {
		for owner := range owners {
			if desired, _, ok := ReplicasOf(model, owner); ok {
				r.Of, r.Replicas = owner, int(desired)
			}
		}
	}
	for _, id := range append([]inventory.EntityID{p.Root}, p.Impact...) {
		switch {
		case id.Kind == kube.KindService:
			r.Services++
		case id.Kind == kube.KindIngress:
			r.Ingresses++
		case IsWorkload(id.Kind) && id != r.Of:
			r.Workloads++
		}
	}
	return r
}

// noteRadius recomputes p's radius and keeps the widest in the journal,
// with the highest pod count seen.
func (p *Incident) noteRadius(model inventory.Reader) {
	p.Radius = radiusOf(model, p)
	peak := max(p.Journal.Widest.Pods, p.Radius.Pods)
	if p.Radius.size() > p.Journal.Widest.size() {
		p.Journal.Widest = p.Radius
	}
	// The pod peak is kept on its own: a wider radius with fewer pods
	// must not make "N pods were affected" understate the pods.
	p.Journal.Widest.Pods = peak
}

// Summary says the radius in one line, such as "Affects 2 services, 1
// ingress and 3 of 4 pods of payments", or "" when it counts nothing.
func (r Radius) Summary() string {
	var parts []string
	add := func(n int, one string) {
		if n == 1 {
			parts = append(parts, "1 "+one)
		} else if n > 1 {
			parts = append(parts, strconv.Itoa(n)+" "+pluralOf(one))
		}
	}
	add(r.Services, "service")
	add(r.Ingresses, "ingress")
	add(r.Workloads, "workload")
	switch {
	case r.Pods > 0 && r.Of.Name != "" && r.Replicas >= r.Pods:
		noun := "pods"
		if r.Replicas == 1 {
			noun = "pod"
		}
		parts = append(parts, strconv.Itoa(r.Pods)+" of "+
			strconv.Itoa(r.Replicas)+" "+noun+" of "+r.Of.Name)
	default:
		add(r.Pods, "pod")
	}
	if len(parts) == 0 {
		return ""
	}
	return "Affects " + joinWords(parts)
}

// pluralOf is the plural of a kind word in Summary.
func pluralOf(word string) string {
	if strings.HasSuffix(word, "s") {
		return word + "es"
	}
	return word + "s"
}

// joinWords joins "a", "b" and "c" as "a, b and c".
func joinWords(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	last := len(parts) - 1
	return strings.Join(parts[:last], ", ") + " and " + parts[last]
}
