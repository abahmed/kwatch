package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// twoOperators puts the gone webhook backends of two operators,
// tracing-operator-webhook and policy-svc, in namespace default, and
// returns the model and its context.
func twoOperators() (*inventory.Model, detection.Context) {
	m := newTestModel()
	for _, name := range []string{"tracing", "policy"} {
		hook := newID(kube.KindValidatingHook, "", name+"-validation")
		put(m, hook, t0, map[string]inventory.Value{
			kube.AttrFailurePolicy: inventory.Text("Ignore"),
			kube.AttrRelease:       inventory.Text(name + "-release"),
		})
		svc := newID(kube.KindService, "default", name+"-svc")
		if name == "tracing" {
			svc.Name = "tracing-operator-webhook"
		}
		link(m, hook, inventory.Serves, svc)
	}
	return m, testDetectorContext(m, t0)
}

// expiredSecret is an expired, unused certificate in default.
func expiredSecret(
	m *inventory.Model, name string, extra map[string]inventory.Value,
) inventory.EntityID {
	id := newID(kube.KindSecret, "default", name)
	attrs := map[string]inventory.Value{
		kube.AttrCertExpiry: inventory.Time(t0.Add(-time.Hour))}
	for k, v := range extra {
		attrs[k] = v
	}
	put(m, id, t0, attrs)
	return id
}

// Two operators' leftovers in one namespace name their own backend; an
// object that shares nothing with either names none.
func TestLeftoversOfTwoOperatorsInOneNamespaceStayApart(t *testing.T) {
	m, ctx := twoOperators()
	tracing := newID(kube.KindService, "default",
		"tracing-operator-webhook")
	policy := newID(kube.KindService, "default", "policy-svc")
	cases := map[inventory.EntityID]inventory.EntityID{
		// named after the operator, without its role word
		expiredSecret(m, "tracing-operator-controller-cert", nil): tracing,
		// the same installation name as the webhook
		expiredSecret(m, "cleanup-tls", map[string]inventory.Value{
			kube.AttrRelease: inventory.Text("x,policy-release")}): policy,
		// nothing in common with either
		expiredSecret(m, "audit.default.svc.tls-pair", nil): {},
	}
	for secret, want := range cases {
		got := Certificate{}.Detect(ctx, entityOf(m, secret))
		if assert.Len(t, got, 1, secret.Name) {
			assert.Equal(t, want, got[0].Leftover, secret.Name)
		}
	}
}

// The Secret cert-manager injects a webhook's CA from is the webhook's,
// whatever its name.
func TestWebhookCASecretIsItsLeftover(t *testing.T) {
	m, ctx := twoOperators()
	hook := newID(kube.KindValidatingHook, "", "policy-validation")
	policy := newID(kube.KindService, "default", "policy-svc")
	put(m, hook, t0, map[string]inventory.Value{
		kube.AttrFailurePolicy: inventory.Text("Ignore"),
		kube.AttrCAFrom:        inventory.Text("certificate:default/ca"),
	})
	byCert := expiredSecret(m, "serving", map[string]inventory.Value{
		kube.AttrCertificateName: inventory.Text("default/ca")})
	other := expiredSecret(m, "unrelated", map[string]inventory.Value{
		kube.AttrCertificateName: inventory.Text("default/other")})

	assert.Equal(t, policy, leftoverOf(ctx, byCert, "default"))
	assert.True(t, leftoverOf(ctx, other, "default").IsZero())
}

func TestNamedAfterNeedsAStemAndASeparator(t *testing.T) {
	assert.True(t, namedAfter("policy-tls", "policy-controller"))
	assert.True(t, namedAfter("policy.default.svc", "policy-webhook"))
	assert.False(t, namedAfter("policyx-tls", "policy-controller"))
	assert.False(t, namedAfter("my-other-tls", "my-webhook"),
		"a short stem names nothing")
}
