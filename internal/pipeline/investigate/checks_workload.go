package investigate

import (
	"context"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func hasWorkload(p incident.Incident, model inventory.Reader) bool {
	w, ok := workloadOf(model, p)
	return ok && model.Exists(w)
}

var rolloutFacts = []labelled{
	{"Revision", kube.AttrRevision},
	{"Current revision", kube.AttrCurrentRevision},
	{"Desired replicas", kube.AttrDesiredReplicas},
	{"Ready replicas", kube.AttrReadyReplicas},
	{"Updated replicas", kube.AttrUpdatedReplicas},
	{"Available replicas", kube.AttrAvailable},
	{"Progress deadline exceeded", kube.AttrProgressDeadline},
}

// diffPaths are the change paths whose values are shown: the image and
// resources. Every other path is named only, so an env value never
// leaves the model.
var diffPaths = []string{"image", "resources"}

// readRollout reads the workload's rollout state and what its recent
// changes touched; for an image or resources change, the value before
// and after.
func readRollout(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	w, _ := workloadOf(s.Model, p)
	e, _ := s.Model.Entity(w)
	sec := newSection(string(w.Kind) + "/" + w.Name)
	addFacts(sec, "", e, rolloutFacts)
	for _, c := range failingConditions(e) {
		sec.fact("Condition", c)
	}
	for _, change := range s.Model.Changes(w,
		p.Opened.Add(-configLookback)) {
		for _, f := range change.Fields {
			sec.fact("Changed at "+change.At.UTC().Format("15:04 UTC"),
				changedField(f))
		}
	}
	return []incident.Section{sec.done()}
}

// changedField names a changed path, with its values only for an image
// or resources path.
func changedField(f inventory.FieldChange) string {
	for _, shown := range diffPaths {
		if strings.Contains(f.Path, shown) {
			return f.Path + ": " + f.Before + " → " + f.After
		}
	}
	return f.Path
}

func isDeleting(p incident.Incident, model inventory.Reader) bool {
	e, ok := model.Entity(p.Root)
	return ok && textOf(e, kube.AttrFinalizers) != "" &&
		textOf(e, kube.AttrDeleting) == "true"
}

// readFinalizer reads the finalizers holding the root and the
// controllers that handle them.
func readFinalizer(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	e, _ := s.Model.Entity(p.Root)
	sec := newSection(p.Root.Name)
	sec.fact("Finalizers", textOf(e, kube.AttrFinalizers))
	sec.fact("Handled by", textOf(e, kube.AttrFinalizerManagers))
	sec.fact("Deleting since", textOf(e, kube.AttrDeletingSince))
	return []incident.Section{sec.done()}
}

func rootExists(p incident.Incident, model inventory.Reader) bool {
	return model.Exists(p.Root)
}

func hasObject(p incident.Incident, model inventory.Reader) bool {
	_, ok := objectOf(model, p)
	return ok
}

// objectOf is what a "kubectl describe" step names: the root when the
// model has it, else the most severe member it has.
func objectOf(
	model inventory.Reader, p incident.Incident,
) (inventory.EntityID, bool) {
	if model.Exists(p.Root) {
		return p.Root, true
	}
	for _, m := range sortedMembers(p) {
		if model.Exists(m.Entity) {
			return m.Entity, true
		}
	}
	return inventory.EntityID{}, false
}

// readObject reads the object's own state: phase, reason, message,
// failing conditions and events, what "kubectl describe" shows.
func readObject(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	id, _ := objectOf(s.Model, p)
	e, _ := s.Model.Entity(id)
	sec := newSection(string(id.Kind) + "/" + id.Name)
	addFacts(sec, "", e, podFacts)
	for _, c := range failingConditions(e) {
		sec.fact("Condition", c)
	}
	for _, line := range notesOf(s.Model, p, []inventory.EntityID{id}) {
		sec.line(line, false)
	}
	return []incident.Section{sec.done()}
}

func hasAutoscaler(p incident.Incident, model inventory.Reader) bool {
	w, ok := workloadOf(model, p)
	return ok && len(relatedOf(model, w, inventory.Scales,
		inventory.Incoming, kube.KindHPA)) > 0
}

var hpaFacts = []labelled{
	{"Current replicas", kube.AttrCurrentReplicas},
	{"Desired replicas", kube.AttrDesiredReplicas},
	{"Min replicas", kube.AttrMinReplicas},
	{"Max replicas", kube.AttrMaxReplicas},
}

// readAutoscaler reads the autoscalers of the workload: replicas and
// the conditions that say whether metrics are available.
func readAutoscaler(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	w, _ := workloadOf(s.Model, p)
	var out []incident.Section
	for _, hpa := range relatedOf(s.Model, w, inventory.Scales,
		inventory.Incoming, kube.KindHPA) {
		e, _ := s.Model.Entity(hpa)
		sec := newSection(hpa.Name)
		addFacts(sec, "", e, hpaFacts)
		for _, c := range failingConditions(e) {
			sec.fact("Condition", c)
		}
		out = append(out, sec.done())
	}
	return out
}

func isJob(p incident.Incident, model inventory.Reader) bool {
	w, ok := workloadOf(model, p)
	return ok && w.Kind == kube.KindJob
}

var jobFacts = []labelled{
	{"Backoff limit", kube.AttrBackoffLimit}, {"Active", kube.AttrActive},
	{"Succeeded", kube.AttrSucceeded}, {"Failed", kube.AttrFailed},
	{"Deadline seconds", kube.AttrActiveDeadline},
}

// readJob reads the Job's run counts and the last exit of its pods.
func readJob(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	w, _ := workloadOf(s.Model, p)
	e, _ := s.Model.Entity(w)
	sec := newSection(w.Name)
	addFacts(sec, "", e, jobFacts)
	for _, pod := range incidentPods(s.Model, p) {
		for _, c := range containersOf(s.Model, pod) {
			ce, _ := s.Model.Entity(c)
			if code := textOf(ce, kube.AttrLastExitCode); code != "" {
				sec.fact(c.Name+": last exit code", code)
			}
		}
	}
	return []incident.Section{sec.done()}
}

func hasQuota(p incident.Incident, model inventory.Reader) bool {
	return len(quotasOf(model, p)) > 0
}

// quotasOf are the namespace's quotas that are exhausted or near their
// limit.
func quotasOf(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, q := range model.EntitiesIn(kube.KindQuota, p.Root.Namespace) {
		e, _ := model.Entity(q)
		if textOf(e, kube.AttrExhausted) != "" ||
			textOf(e, kube.AttrQuotaNearLimit) != "" {
			out = append(out, q)
		}
	}
	sortIDs(out)
	return out
}

// readQuota reads the usage of the quotas near or at their hard limit.
func readQuota(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, q := range quotasOf(s.Model, p) {
		e, _ := s.Model.Entity(q)
		sec := newSection(q.Name)
		sec.fact("Exhausted", textOf(e, kube.AttrExhausted))
		sec.fact("Near the limit", textOf(e, kube.AttrQuotaNearLimit))
		out = append(out, sec.done())
	}
	return out
}

func pullsImage(p incident.Incident, model inventory.Reader) bool {
	return isRegistryRoot(p) && hasPods(p, model)
}

// readImage reads each pod's images and its pull secrets, and whether
// each Secret exists. Secret values are never read.
func readImage(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, pod := range incidentPods(s.Model, p) {
		e, _ := s.Model.Entity(pod)
		sec := newSection(pod.Name)
		for _, c := range containersOf(s.Model, pod) {
			ce, _ := s.Model.Entity(c)
			sec.fact(c.Name+": image", textOf(ce, kube.AttrImage))
		}
		for _, name := range pullSecrets(e) {
			id := inventory.CoreID(kube.KindSecret, pod.Namespace, name)
			sec.fact("Pull secret", name+existence(s.Model, id))
		}
		out = append(out, sec.done())
	}
	return out
}

// pullSecrets are the pull secret names the pod and its account name.
func pullSecrets(pod inventory.Entity) []string {
	var out []string
	for _, attr := range []string{kube.AttrPullSecrets,
		kube.AttrAccountPullSecrets} {
		for _, name := range strings.Split(textOf(pod, attr), ",") {
			if name = strings.TrimSpace(name); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func hasCertificate(p incident.Incident, model inventory.Reader) bool {
	return len(certificatesOf(model, p)) > 0
}

// certificatesOf are the root and the Secrets of the incident's
// Ingresses that carry a certificate expiry.
func certificatesOf(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	candidates := []inventory.EntityID{p.Root}
	for _, ing := range ingressesOf(model, p) {
		candidates = append(candidates, relatedOf(model, ing,
			inventory.References, inventory.Outgoing, kube.KindSecret)...)
	}
	var out []inventory.EntityID
	for _, id := range firstUnique(candidates, len(candidates)) {
		if e, ok := model.Entity(id); ok &&
			textOf(e, kube.AttrCertExpiry) != "" {
			out = append(out, id)
		}
	}
	return out
}

// readCertificates reads when each certificate expires.
func readCertificates(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	var out []incident.Section
	for _, id := range certificatesOf(s.Model, p) {
		e, _ := s.Model.Entity(id)
		sec := newSection(string(id.Kind) + "/" + id.Name)
		sec.fact("Expires", textOf(e, kube.AttrCertExpiry))
		sec.fact("Type", textOf(e, kube.AttrSecretType))
		out = append(out, sec.done())
	}
	return out
}

// readConfigKeys adds, for each ConfigMap or Secret the incident
// depends on, whether it exists and its key names. Values are never
// read.
func readConfigKeys(
	_ context.Context, s Sources, p incident.Incident,
) []incident.Section {
	sec := newSection("")
	for _, obj := range configObjects(s.Model, p) {
		label := string(obj.Kind) + " " + obj.Name
		e, ok := s.Model.Entity(obj)
		if !ok {
			sec.fact(label, "does not exist in "+obj.Namespace)
			continue
		}
		sec.fact(label+": keys", textOf(e, kube.AttrKeys))
	}
	return []incident.Section{sec.done()}
}
