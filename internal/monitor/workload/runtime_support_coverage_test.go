package workload

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestSourceConfigurationWiresEveryRuntime(t *testing.T) {
	sink := &replicaSetSinkRecorder{}
	wiring := NewSourceConfiguration(
		NewDeploymentRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
			time.Now),
		NewReplicaSetRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
			time.Now),
		NewDaemonSetRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
			nil, time.Now),
		NewStatefulSetRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
			time.Now),
		NewJobRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink, time.Now),
		NewCronJobRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink,
			time.Now),
		NewHPARuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink, time.Now),
		NewPDBRuntimeWithRuntimeConfig(config.RuntimeConfig{}, sink, time.Now),
	)
	if err := wiring.ConfigureSources(Sources{}); err != nil {
		t.Fatalf("ConfigureSources() error = %v", err)
	}
	if err := wiring.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second ConfigureSources() succeeded")
	}
	if err := (*SourceConfiguration)(nil).ConfigureSources(Sources{}); err == nil {
		t.Fatal("nil source configuration succeeded")
	}
}

func TestRuntimeSupportHelpersCoverMaintenanceAndSustain(t *testing.T) {
	if adaptiveSustained(10, true, 10, 1) != 11*time.Minute {
		t.Fatal("adaptive sustain did not add bounded grace")
	}
	if adaptiveSustained(0, true, 10, 1) != 0 {
		t.Fatal("disabled sustain did not remain zero")
	}
	if adaptiveSustained(10, false, 10, 1) != 10*time.Minute {
		t.Fatal("non-adaptive sustain changed the configured duration")
	}
	fixedNow := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	daemonSet := &DaemonSetRuntime{
		support: newRuntimeSupportAt(config.RuntimeConfig{}, nil,
			func() time.Time { return fixedNow }),
	}
	unsettled := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Generation: 2,
	}, Status: appsv1.DaemonSetStatus{
		DesiredNumberScheduled: 2,
		NumberUnavailable:      1,
	}}
	if daemonSet.unavailableSustained(unsettled,
		fixedNow.Add(-time.Minute)) {
		t.Fatal("unsettled daemonset became sustained too early")
	}
	settled := unsettled.DeepCopy()
	settled.Status.ObservedGeneration = settled.Generation
	settled.Status.UpdatedNumberScheduled = settled.Status.DesiredNumberScheduled
	if !daemonSet.unavailableSustained(settled,
		fixedNow.Add(-20*time.Minute)) {
		t.Fatal("settled daemonset did not become sustained")
	}
	if inMaintenance(config.MaintenanceConfig{}, nil, time.Time{}) {
		t.Fatal("disabled maintenance was active")
	}
	maintenance := config.MaintenanceConfig{
		Enabled: true, Annotation: "maint", UntilAnnotation: "until",
	}
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if !inMaintenance(maintenance, map[string]string{
		"until": now.Add(time.Hour).Format(time.RFC3339),
	}, now) {
		t.Fatal("future maintenance window was inactive")
	}
	if !inMaintenance(config.MaintenanceConfig{
		Enabled: true, Annotation: "maint",
	}, map[string]string{"maint": "TRUE"}, now) {
		t.Fatal("active maintenance annotation was inactive")
	}
	if inMaintenance(maintenance, map[string]string{"until": "bad"}, now) {
		t.Fatal("malformed maintenance window was active")
	}
	first := &firstSeen{}
	if got := first.mark("key", now); !got.Equal(now) {
		t.Fatal("firstSeen did not record initial time")
	}
	if got := first.mark("key", now.Add(time.Hour)); !got.Equal(now) {
		t.Fatal("firstSeen changed existing time")
	}
	first.clear("key")
	if observations(nil) != nil {
		t.Fatal("nil observation produced a slice")
	}
	observation := &model.Observation{}
	if len(observations(observation)) != 1 {
		t.Fatal("observation was not wrapped")
	}
	sink := &replicaSetSinkRecorder{}
	support := newRuntimeSupportAt(config.RuntimeConfig{}, sink, time.Now)
	if support.maintenance(nil) {
		t.Fatal("disabled runtime maintenance was active")
	}
	support.observe(observation)
	support.reconcile(model.ObjectRef{Kind: "deployment"},
		[]*model.Observation{observation, nil})
	support.reconcileGone(model.ObjectRef{Kind: "deployment"})
	if sink.reconciledSubject.Kind != "deployment" {
		t.Fatalf("reconciled subject = %+v", sink.reconciledSubject)
	}
	if err := processKey[string, string](
		&support, "bad/key/extra", "deployment", false, nil,
		func() (string, bool) { return "", false },
		func(string, string, string) (string, error) { return "", nil },
		func(model.ObjectRef) {},
		func(model.ObjectRef, string) error { return nil },
	); err == nil {
		t.Fatal("invalid generic key succeeded")
	}
	if err := processKey[string, string](
		&support, "apps/object", "deployment", false, nil,
		func() (string, bool) { return "", false },
		func(string, string, string) (string, error) { return "", nil },
		func(model.ObjectRef) {},
		func(model.ObjectRef, string) error { return nil },
	); err != nil {
		t.Fatalf("unavailable generic source returned error: %v", err)
	}
	if Descriptor().Name == "" {
		t.Fatal("workload descriptor has no name")
	}
}
