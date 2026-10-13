package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// attrs is a set of attributes of one entity in a test.
type attrs map[string]inventory.Value

// setAttrs records attributes for id.
func (f *fixture) setAttrs(id inventory.EntityID, a attrs) {
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: a})
}

// placeFixture is a deployment with one pod whose container app has the
// given attributes.
func placeFixture(t *testing.T, a attrs) (*fixture, Cause) {
	t.Helper()
	f := newFixture(t)
	pods := f.workload("shop", "api", 1)
	f.setAttrs(containerOf(pods[0]), a)
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	return f, Cause{Root: root, Covers: pods}
}

func wantPlace(
	t *testing.T, got *detection.Place, container, path, value string,
) {
	t.Helper()
	if got == nil {
		t.Fatalf("no place, want %s %s (%s)", container, path, value)
	}
	if got.Container != container || got.Path != path || got.Value != value {
		t.Fatalf("place = %+v, want container %q path %q value %q",
			*got, container, path, value)
	}
}

func TestMemoryLimitPlaceNamesTheLimit(t *testing.T) {
	started := t0.Add(-time.Minute)
	f, c := placeFixture(t, attrs{
		kube.AttrMemoryLimit: inventory.Number(256 << 20),
		kube.AttrLastStarted: inventory.Time(started),
		kube.AttrLastFinished: inventory.Time(
			started.Add(10 * time.Second)),
	})
	c.Row = "memory-limit-too-low"
	place := f.snapshot().placeOf(c, nil)
	wantPlace(t, place, "app", "resources.limits.memory", "256Mi")
	if place.Object.Name != "api" || place.Object.Kind != kube.KindDeployment {
		t.Fatalf("object = %v, want deployment api", place.Object)
	}
}

func TestEphemeralLimitPlace(t *testing.T) {
	f, c := placeFixture(t, attrs{
		kube.AttrEphemeralLimit: inventory.Number(1 << 30)})
	c.Row = "ephemeral-limit-too-low"
	wantPlace(t, f.snapshot().placeOf(c, nil), "app",
		"resources.limits.ephemeral-storage", "1Gi")
}

func TestLivenessPlaceNamesTimingFields(t *testing.T) {
	f, c := placeFixture(t, attrs{
		kube.AttrLivenessDelay:    inventory.Number(10),
		kube.AttrLivenessPeriod:   inventory.Number(10),
		kube.AttrLivenessFailures: inventory.Number(3)})
	c.Row = "liveness-shorter-than-start"
	wantPlace(t, f.snapshot().placeOf(c, nil), "app", "livenessProbe",
		"initialDelaySeconds 10, periodSeconds 10, failureThreshold 3")
}

func TestStartupBudgetPlacePrefersTheStartupProbe(t *testing.T) {
	a := attrs{kube.AttrProbeBudget: inventory.Number(40)}
	for _, probe := range []string{"startup", "readiness"} {
		a[kube.ProbeAttr(probe, "delay")] = inventory.Number(5)
		a[kube.ProbeAttr(probe, "period")] = inventory.Number(10)
		a[kube.ProbeAttr(probe, "failures")] = inventory.Number(3)
	}
	f, c := placeFixture(t, a)
	c.Row = "startup-budget-too-short"
	wantPlace(t, f.snapshot().placeOf(c, nil), "app", "startupProbe",
		"initialDelaySeconds 5, periodSeconds 10, failureThreshold 3")
}

func TestProbePortPlaceNamesTheUndeclaredPort(t *testing.T) {
	f, c := placeFixture(t, attrs{
		kube.AttrContainerPorts:                inventory.Text("8080"),
		kube.AttrProbePorts:                    inventory.Text("8081"),
		kube.ProbeAttr("readiness", "port"):    inventory.Text("8081"),
		kube.ProbeAttr("readiness", "handler"): inventory.Text("httpGet"),
		kube.ProbeAttr("liveness", "port"):     inventory.Text("8080"),
		kube.ProbeAttr("liveness", "handler"):  inventory.Text("tcpSocket"),
	})
	c.Row = "probe-port-mismatch"
	wantPlace(t, f.snapshot().placeOf(c, nil), "app",
		"readinessProbe.httpGet.port", "8081")
}

func TestPlaceIsNilWhenAttributesAreMissing(t *testing.T) {
	f, c := placeFixture(t, attrs{})
	for _, row := range []string{"memory-limit-too-low",
		"liveness-kills-before-ready", "startup-budget-too-short",
		"probe-port-mismatch", "ephemeral-limit-too-low"} {
		c.Row = row
		if got := f.snapshot().placeOf(c, nil); got != nil {
			t.Errorf("%s: place = %+v, want none", row, *got)
		}
	}
}

func TestMissingSecretPlaceNamesTheReferenceNotTheValue(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "stripe-api")
	f, c := placeFixture(t, attrs{
		kube.RefAttr(kube.KindSecret, "stripe-api"): inventory.Text(
			"env[STRIPE_KEY].valueFrom.secretKeyRef")})
	c.Row, c.Mode, c.Root = "config-missing-or-changed", ModeMissing, secret
	wantPlace(t, f.snapshot().placeOf(c, nil), "app",
		"env[STRIPE_KEY].valueFrom.secretKeyRef", "secret stripe-api")
}

func TestMissingConfigPlaceIgnoresChangedData(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "stripe-api")
	f, c := placeFixture(t, attrs{
		kube.RefAttr(kube.KindSecret, "stripe-api"): inventory.Text(
			"envFrom.secretRef")})
	c.Row, c.Mode, c.Root = "config-missing-or-changed",
		ModeSpecChanged, secret
	if got := f.snapshot().placeOf(c, nil); got != nil {
		t.Fatalf("a changed secret gave place %+v", *got)
	}
}

func TestMissingSecretPlaceFindsAVolumeOfThePod(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "certs")
	f, c := placeFixture(t, attrs{})
	f.setAttrs(c.Covers[0], attrs{
		kube.RefAttr(kube.KindSecret, "certs"): inventory.Text(
			"volumes[tls].secret")})
	c.Row, c.Mode, c.Root = "used-missing", ModeMissing, secret
	place := f.snapshot().placeOf(c, nil)
	wantPlace(t, place, "", "volumes[tls].secret", "secret certs")
	if place.Object.Name != "api" {
		t.Fatalf("object = %v, want deployment api", place.Object)
	}
}

func TestObjectPlaces(t *testing.T) {
	f := newFixture(t)
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	pdb := inventory.CoreID(kube.KindPDB, "shop", "web")
	pvc := inventory.CoreID(kube.KindPVC, "shop", "data")
	ingress := inventory.CoreID(kube.KindIngress, "shop", "front")
	missing := inventory.CoreID(kube.KindService, "shop", "web-v2")
	class := inventory.CoreID(kube.KindStorageClass, "", "fast")
	f.setAttrs(hpa, attrs{kube.AttrMaxReplicas: inventory.Number(6)})
	f.setAttrs(pdb, attrs{kube.AttrMinAvailable: inventory.Text("2")})
	f.setAttrs(pvc, attrs{kube.AttrRequested: inventory.Number(20 << 30)})
	s := f.snapshot()
	cases := []struct {
		name string
		c    Cause
		path string
		want string
	}{
		{"hpa", Cause{Row: "autoscaling-limit-unavailable", Root: hpa},
			"spec.maxReplicas", "6"},
		{"pdb", Cause{Row: "budget-blocks-drain", Root: pdb},
			"spec.minAvailable", "2"},
		{"pvc size", Cause{Row: "claim-not-usable", Root: pvc,
			Mode: detection.ModeVolumeFull},
			"spec.resources.requests.storage", "20Gi"},
		{"ingress backend", Cause{Row: "routed-missing", Root: missing,
			Covers: []inventory.EntityID{ingress}},
			"spec.rules[].http.paths[].backend.service.name", "web-v2"},
		{"storage class", Cause{Row: "storage-class-missing", Root: class,
			Covers: []inventory.EntityID{pvc}},
			"spec.storageClassName", "fast"},
	}
	for _, tc := range cases {
		got := s.placeOf(tc.c, nil)
		wantPlace(t, got, "", tc.path, tc.want)
		if got.Object.Name == "" {
			t.Errorf("%s: no object", tc.name)
		}
	}
	inodes := Cause{Row: "claim-not-usable", Root: pvc,
		Mode: detection.ModeVolumeInodes}
	if got := s.placeOf(inodes, nil); got != nil {
		t.Errorf("full inodes gave place %+v, want none", *got)
	}
}

func TestSchedulingPlaceNamesTolerationsAndSelector(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 1)
	f.setAttrs(pods[0], attrs{kube.AttrSchedulingSpec: inventory.Text(
		`{"sel":{"disk":"ssd"},"tol":[{"k":"team","o":"Equal",` +
			`"v":"a","e":"NoSchedule"}]}`)})
	s := f.snapshot()
	taint := Cause{Row: "scheduler-capacity", Covers: pods,
		Root: inventory.CoreID(KindScheduling, "", "had untolerated taint")}
	wantPlace(t, s.placeOf(taint, nil), "", "spec.template.spec.tolerations",
		"team=a:NoSchedule")
	selector := Cause{Row: "scheduler-capacity", Covers: pods,
		Root: inventory.CoreID(KindScheduling, "",
			"didn't match Pod's node affinity/selector")}
	wantPlace(t, s.placeOf(selector, nil), "",
		"spec.template.spec.nodeSelector", "disk=ssd")
	short := Cause{Row: "scheduler-capacity", Covers: pods,
		Root: inventory.CoreID(KindScheduling, "", "Insufficient cpu")}
	if got := s.placeOf(short, nil); got != nil {
		t.Errorf("a CPU shortage gave place %+v, want none", *got)
	}
}

func TestChangePlaceNamesTheFieldAndHidesSecretsAndEnvValues(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	field := func(path, after string) inventory.Change {
		return inventory.Change{Entity: root, Fields: []inventory.FieldChange{
			{Path: path, After: after}}}
	}
	s := newFixture(t).snapshot()
	wantPlace(t, s.placeOf(Cause{Root: root},
		ptr(field("containers[app].image", "api:2"))), "app", "image", "api:2")
	wantPlace(t, s.placeOf(Cause{Root: root},
		ptr(field("containers[app].resources.limits.memory", "256Mi"))),
		"app", "resources.limits.memory", "256Mi")
	wantPlace(t, s.placeOf(Cause{Root: root},
		ptr(field("containers[app].env.DB_PASSWORD", "hunter2"))),
		"app", "env[DB_PASSWORD]", "")
	wantPlace(t, s.placeOf(Cause{Root: root},
		ptr(field("spec.replicas", "0"))), "", "spec.replicas", "0")
	for _, path := range []string{"metadata.deletionTimestamp",
		"template.restartedAt", "data.token"} {
		got := s.placeOf(Cause{Root: root}, ptr(field(path, "x")))
		if got != nil {
			t.Errorf("%s gave place %+v, want none", path, *got)
		}
	}
	secret := inventory.CoreID(kube.KindSecret, "shop", "creds")
	change := inventory.Change{Entity: secret, Fields: []inventory.FieldChange{
		{Path: "spec.replicas", After: "x"}}}
	if got := s.placeOf(Cause{Root: secret}, &change); got != nil {
		t.Errorf("a secret gave place %+v, want none", *got)
	}
}

func ptr[T any](v T) *T { return &v }

func TestRecordKeepsThePlace(t *testing.T) {
	f, c := placeFixture(t, attrs{
		kube.AttrEphemeralLimit: inventory.Number(1 << 30)})
	c.Row = "ephemeral-limit-too-low"

	record := f.snapshot().Record(c)

	wantPlace(t, record.Place, "app", "resources.limits.ephemeral-storage",
		"1Gi")
}
