package kube_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeDials runs the first automatic probe round over the Services the
// model holds and returns the addresses dialed. A round that dials
// nothing submits nothing, so the round is also given a short time.
func probeDials(t *testing.T, model *inventory.Model) []string {
	t.Helper()
	var mu sync.Mutex
	var dialed []string
	ctx, cancel := context.WithTimeout(context.Background(),
		500*time.Millisecond)
	defer cancel()
	prober := kube.NewActiveProber(kube.ActiveProbeConfig{
		AutoServices: true, Model: model, Timeout: 200 * time.Millisecond,
		Now:    fixedTime,
		Submit: func(context.Context, ...inventory.Observation) {},
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			mu.Lock()
			defer mu.Unlock()
			dialed = append(dialed, address)
			return nil, errors.New("refused")
		},
	})
	prober.Run(ctx)
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), dialed...)
}

func addService(
	t *testing.T, model *inventory.Model, name string,
	attrs map[string]inventory.Value,
) {
	t.Helper()
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: inventory.CoreID(kube.KindService, "ns", name),
		Attributes: attrs,
	})
	require.NoError(t, err)
}

func TestAutoServiceProbeDialsTheFirstTCPPort(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	addService(t, model, "dns", map[string]inventory.Value{
		kube.AttrPorts: inventory.Text("53/UDP->53,53/TCP->53")})
	addService(t, model, "statsd", map[string]inventory.Value{
		kube.AttrPorts: inventory.Text("8125/UDP->8125")})
	assert.Equal(t, []string{"dns.ns.svc:53"}, probeDials(t, model))
}

func TestAutoServiceProbeSkipsExternalNameServices(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	addService(t, model, "outside", map[string]inventory.Value{
		kube.AttrPorts:       inventory.Text("443/TCP->443"),
		kube.AttrServiceType: inventory.Text("ExternalName"),
	})
	assert.Empty(t, probeDials(t, model))
}

func TestAutoServiceProbeSkipsHeadlessServices(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Ports: []corev1.ServicePort{{
				Port: 5432, Protocol: corev1.ProtocolTCP}}},
	}
	desc, ok := kube.ServiceSchema{}.Describe(svc)
	require.True(t, ok)
	model := inventory.NewModel(inventory.Options{})
	addService(t, model, "db", desc.Attributes)
	assert.Empty(t, probeDials(t, model))
}

func TestAutoServiceProbeHonoursTheSkipAnnotation(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "quiet", Namespace: "ns",
			Annotations: map[string]string{"kwatch.io/skip-probe": "true"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
			Port: 80, Protocol: corev1.ProtocolTCP}}},
	}
	desc, ok := kube.ServiceSchema{}.Describe(svc)
	require.True(t, ok)
	model := inventory.NewModel(inventory.Options{})
	addService(t, model, "quiet", desc.Attributes)
	addService(t, model, "loud", map[string]inventory.Value{
		kube.AttrPorts: inventory.Text("80/TCP->80")})
	assert.Equal(t, []string{"loud.ns.svc:80"}, probeDials(t, model))
}

func TestAutoServiceProbeIsCappedAtTwoHundred(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	for i := range 230 {
		addService(t, model, fmt.Sprintf("svc-%03d", i),
			map[string]inventory.Value{
				kube.AttrPorts: inventory.Text("80/TCP->80")})
	}
	dialed := probeDials(t, model)
	assert.Len(t, dialed, 200)
	assert.Contains(t, dialed, "svc-000.ns.svc:80",
		"the same Services are chosen every round (name order)")
	assert.NotContains(t, dialed, "svc-229.ns.svc:80")
}
