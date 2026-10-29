package investigate

import (
	"context"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func readsNode(p incident.Incident, _ inventory.Reader) bool {
	return isNodeRoot(p) || killingNode(p) != ""
}

// readNodePods lists the pods on the node and their state.
func readNodePods(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	node := targetNode(p)
	sec := newSection(node.Name)
	pods := s.Model.Related(node, inventory.RunsOn, inventory.Incoming)
	sortIDs(pods)
	sec.fact("Pods", strconv.Itoa(len(pods)))
	for _, pod := range pods[:min(len(pods), maxNodePods)] {
		e, _ := s.Model.Entity(pod)
		line := pod.Namespace + "/" + pod.Name + " " +
			textOf(e, kube.AttrPhase)
		if ready := textOf(e, kube.AttrReady); ready != "" {
			line += ", ready " + ready
		}
		sec.line(line, false)
	}
	return []incident.Section{sec.done()}
}

// claimsOf are the root claim or the claims the incident's pods mount.
func claimsOf(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	if p.Root.Kind == kube.KindPVC && model.Exists(p.Root) {
		return []inventory.EntityID{p.Root}
	}
	var out []inventory.EntityID
	for _, m := range sortedMembers(p) {
		if m.Entity.Kind == kube.KindPVC && model.Exists(m.Entity) {
			out = append(out, m.Entity)
		}
	}
	for _, pod := range incidentPods(model, p) {
		out = append(out, relatedOf(model, pod, inventory.Mounts,
			inventory.Outgoing, kube.KindPVC)...)
	}
	return firstUnique(out, maxReportPods)
}

func hasClaims(p incident.Incident, model inventory.Reader) bool {
	return len(claimsOf(model, p)) > 0
}

var claimFacts = []labelled{
	{"Phase", kube.AttrPhase}, {"Access modes", kube.AttrAccessModes},
	{"Requested", kube.AttrRequested}, {"Capacity", kube.AttrCapacity},
}

// readVolumes reads each claim: phase, class and whether it exists,
// provisioner, bound volume, access modes, size and events.
func readVolumes(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, claim := range claimsOf(s.Model, p) {
		e, ok := s.Model.Entity(claim)
		if !ok {
			continue
		}
		sec := newSection(claim.Name)
		addFacts(sec, "", e, claimFacts)
		addClassFacts(sec, s.Model, claim)
		for _, pv := range relatedOf(s.Model, claim, inventory.References,
			inventory.Outgoing, kube.KindPV) {
			sec.fact("Volume", pv.Name+existence(s.Model, pv))
		}
		for _, line := range notesOf(s.Model, p,
			[]inventory.EntityID{claim}) {
			sec.line(line, true)
		}
		out = append(out, sec.done())
	}
	return out
}

func addClassFacts(
	sec *section, model inventory.Reader, claim inventory.EntityID,
) {
	for _, class := range relatedOf(model, claim, inventory.References,
		inventory.Outgoing, kube.KindStorageClass) {
		sec.fact("StorageClass", class.Name+existence(model, class))
		if e, ok := model.Entity(class); ok {
			sec.fact("Provisioner", textOf(e, kube.AttrProvisioner))
		}
	}
}

// existence is ", which does not exist" for an object the model lacks.
func existence(model inventory.Reader, id inventory.EntityID) string {
	if model.Exists(id) {
		return ""
	}
	return ", which does not exist"
}

// servicesOf are the root Service or the Services selecting the pods.
func servicesOf(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	if p.Root.Kind == kube.KindService {
		if !model.Exists(p.Root) {
			return nil
		}
		return []inventory.EntityID{p.Root}
	}
	var out []inventory.EntityID
	for _, pod := range incidentPods(model, p) {
		out = append(out, relatedOf(model, pod, inventory.Selects,
			inventory.Incoming, kube.KindService)...)
	}
	if p.Root.Kind == kube.KindIngress {
		out = append(out, relatedOf(model, p.Root, inventory.RoutesTo,
			inventory.Outgoing, kube.KindService)...)
	}
	return firstUnique(out, maxReportPods)
}

func hasServices(p incident.Incident, model inventory.Reader) bool {
	return len(servicesOf(model, p)) > 0
}

// readServices reads each Service: type, selector, ports, the pods it
// selects and its ready endpoints.
func readServices(
	ctx context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, svc := range servicesOf(s.Model, p) {
		out = append(out, serviceSection(ctx, s, svc))
	}
	return out
}

func serviceSection(
	ctx context.Context, s Sources, svc inventory.EntityID,
) incident.Section {
	sec := newSection(svc.Namespace + "/" + svc.Name)
	e, ok := s.Model.Entity(svc)
	if !ok {
		sec.Status = incident.SectionAbsent
		return sec.done()
	}
	sec.fact("Type", textOf(e, kube.AttrServiceType))
	sec.fact("Selector", textOf(e, kube.AttrSelector))
	sec.fact("Ports", textOf(e, kube.AttrPorts))
	sec.fact("Selected pods", strconv.Itoa(len(relatedOf(s.Model, svc,
		inventory.Selects, inventory.Outgoing, kube.KindPod))))
	if ready, ok := readyEndpoints(ctx, s, svc); ok {
		sec.fact("Ready endpoints", strconv.Itoa(ready))
	}
	return sec.done()
}

// readyEndpoints counts a Service's ready endpoints from the model, else
// from the API.
func readyEndpoints(
	ctx context.Context, s Sources, svc inventory.EntityID,
) (int, bool) {
	ready, ok := modelEndpoints(s.Model, svc)
	if !ok && s.Endpoints != nil {
		ready, ok = s.Endpoints(ctx, svc)
	}
	return ready, ok
}

// ingressesOf are the root Ingress, the Ingresses using the root Secret
// and those routing to the incident's Services.
func ingressesOf(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	if p.Root.Kind == kube.KindIngress {
		return []inventory.EntityID{p.Root}
	}
	var out []inventory.EntityID
	if p.Root.Kind == kube.KindSecret {
		out = relatedOf(model, p.Root, inventory.References,
			inventory.Incoming, kube.KindIngress)
	}
	for _, svc := range servicesOf(model, p) {
		out = append(out, relatedOf(model, svc, inventory.RoutesTo,
			inventory.Incoming, kube.KindIngress)...)
	}
	return firstUnique(out, maxReportPods)
}

func hasIngresses(p incident.Incident, model inventory.Reader) bool {
	return len(ingressesOf(model, p)) > 0
}

// readIngresses reads each Ingress and the chain behind it: class, the
// Services it routes to and their ready endpoints.
func readIngresses(
	ctx context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, ing := range ingressesOf(s.Model, p) {
		sec := newSection(ing.Namespace + "/" + ing.Name)
		e, _ := s.Model.Entity(ing)
		sec.fact("Class", textOf(e, kube.AttrIngressClass))
		for _, svc := range relatedOf(s.Model, ing, inventory.RoutesTo,
			inventory.Outgoing, kube.KindService) {
			value := svc.Name + existence(s.Model, svc)
			if ready, ok := readyEndpoints(ctx, s, svc); ok {
				value += ", " + strconv.Itoa(ready) + " ready endpoints"
			}
			sec.fact("Backend", value)
		}
		for _, secret := range relatedOf(s.Model, ing, inventory.References,
			inventory.Outgoing, kube.KindSecret) {
			sec.fact("TLS Secret", secret.Name+existence(s.Model, secret))
		}
		out = append(out, sec.done())
	}
	return out
}

// controlPlaneLabels are the labels the pods of the cluster components
// kwatch models as single entities carry.
var controlPlaneLabels = map[inventory.EntityID]string{
	kube.ClusterDNS:        "k8s-app=kube-dns",
	kube.Scheduler:         "component=kube-scheduler",
	kube.ControllerManager: "component=kube-controller-manager",
}

func isControlPlaneRoot(p incident.Incident, _ inventory.Reader) bool {
	_, pods := controlPlaneLabels[p.Root]
	return pods || p.Root == kube.APIServer || p.Root == kube.Etcd
}

// readControlPlane reads the API server's readiness checks for an API
// server or etcd root, and the component's pods with their recent
// output for the others.
func readControlPlane(
	ctx context.Context, s Sources, p incident.Incident,
) []incident.Section {
	label, pods := controlPlaneLabels[p.Root]
	if !pods {
		return []incident.Section{readyzSection(ctx, s)}
	}
	sec := newSection(p.Root.Name)
	var first []inventory.EntityID
	for _, pod := range s.Model.EntitiesIn(kube.KindPod, "kube-system") {
		e, _ := s.Model.Entity(pod)
		if !hasLabel(textOf(e, kube.AttrLabels), label) {
			continue
		}
		sec.fact(pod.Name, textOf(e, kube.AttrPhase)+", ready "+
			textOf(e, kube.AttrReady))
		if len(first) == 0 {
			first = containersOf(s.Model, pod)
		}
	}
	out := []incident.Section{sec.done()}
	if s.Tail != nil && len(first) > 0 {
		out = append(out, readTail(ctx, s, first[0], false))
	}
	return out
}

func hasLabel(labels, label string) bool {
	for _, l := range strings.Split(labels, ",") {
		if strings.TrimSpace(l) == label {
			return true
		}
	}
	return false
}

// readyzSection reads /readyz?verbose; failing checks are matched.
func readyzSection(ctx context.Context, s Sources) incident.Section {
	sec := newSection("/readyz")
	if s.Readyz == nil {
		sec.Status, sec.Reason = incident.SectionDenied, incident.ReasonRBAC
		return sec.done()
	}
	lines, err := s.Readyz(ctx)
	if err != nil {
		sec.Status, sec.Reason = readStatus(ctx, err)
		return sec.done()
	}
	for _, line := range lines {
		sec.line(line, strings.HasPrefix(line, "[-]"))
	}
	return sec.done()
}
