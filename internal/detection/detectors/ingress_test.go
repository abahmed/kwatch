package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestIngressReportsMissingTLSSecret(t *testing.T) {
	m := newTestModel()
	ing := newID(kube.KindIngress, "ns", "web")
	put(m, ing, t0, nil)
	link(m, ing, inventory.References,
		inventory.CoreID(kube.KindSecret, "ns", "web-tls"))

	// A certificate controller creates the Secret after the Ingress.
	early := evaluate(Ingress{}, m, t0.Add(DefaultTLSSecretGrace-time.Second),
		ing, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, time.Second, early.RecheckAfter)

	eval := evaluate(Ingress{}, m, t0.Add(DefaultTLSSecretGrace), ing, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Ingress.TLSSecretMissing", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "web-tls")

	put(m, inventory.CoreID(kube.KindSecret, "ns", "web-tls"), t0, nil)
	assert.Empty(t, evaluate(Ingress{}, m, t0.Add(time.Hour), ing,
		nil).Findings)
}

func TestIngressReportsMissingClass(t *testing.T) {
	m := newTestModel()
	ing := newID(kube.KindIngress, "ns", "web")
	put(m, ing, t0, map[string]inventory.Value{
		kube.AttrIngressClass: inventory.Text("nginx"),
	})

	eval := evaluate(Ingress{}, m, t0, ing, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Ingress.ClassMissing", string(eval.Findings[0].Mode))

	unsynced := func(kind inventory.Kind) bool {
		return kind != kube.KindIngressClass
	}
	assert.Empty(t, evaluate(Ingress{}, m, t0, ing, unsynced).Findings)

	put(m, inventory.CoreID(kube.KindIngressClass, "", "nginx"), t0, nil)
	assert.Empty(t, evaluate(Ingress{}, m, t0, ing, nil).Findings)
}

func TestIngressWithoutClassNameIsNotJudged(t *testing.T) {
	m := newTestModel()
	ing := newID(kube.KindIngress, "ns", "web")
	put(m, ing, t0, nil)

	assert.Empty(t, evaluate(Ingress{}, m, t0, ing, nil).Findings)
}
