package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func customResource(kind knowledge.Kind, custom bool,
) (*knowledge.Model, knowledge.EntityID) {
	m := newTestModel()
	id := newID(kind, "default", "thing")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrCustom: knowledge.Bool(custom),
	})
	return m, id
}

func TestCustomFailingCondition(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Ready", "False", "Broken", "boom", t0)
	setCondition(m, id, "Degraded", "True", "", "", t0.Add(time.Minute))

	early := evaluate(Custom{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Signals)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	got := evaluate(Custom{}, m, t0.Add(2*time.Minute), id, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonCustomResourceFailure, got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
	assert.Equal(t, "Reports Degraded=True, Ready=False (Broken)",
		got[0].Summary)
}

func TestCustomNotReadyWithMessage(t *testing.T) {
	m := newTestModel()
	id := newID("widget", "default", "thing")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrCustom:  knowledge.Bool(true),
		kube.AttrReady:   knowledge.Bool(false),
		kube.AttrMessage: knowledge.Text("waiting"),
	})
	got := evaluate(Custom{}, m, t0.Add(time.Hour), id, nil).Signals
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "not ready")
	assert.Equal(t, "waiting", got[0].Evidence[0].Value)
}

func TestCustomQuiet(t *testing.T) {
	now := t0.Add(time.Hour)
	m, id := customResource("widget", false)
	setCondition(m, id, "Ready", "False", "", "", t0)
	assert.Empty(t, evaluate(Custom{}, m, now, id, nil).Signals,
		"not a custom resource")

	m, id = customResource("widget", true)
	setCondition(m, id, "Ready", "True", "", "", t0)
	assert.Empty(t, evaluate(Custom{}, m, now, id, nil).Signals)

	assert.Equal(t, "custom-resource", Custom{}.Name())
	assert.Equal(t, []knowledge.Kind{signal.AnyKind}, Custom{}.Kinds())
}

func TestCustomReasonByKind(t *testing.T) {
	const pf = constant.ReasonAPIPriorityAndFairnessFailure
	tests := []struct {
		kind knowledge.Kind
		want string
	}{
		{"apiservice", constant.ReasonAPIServiceFailure},
		{"volumesnapshot", constant.ReasonVolumeSnapshotFailure},
		{"certificatesigningrequest",
			constant.ReasonCertificateSigningRequestFailure},
		{"flowschema", pf},
		{"prioritylevelconfiguration", pf},
		{"validatingadmissionpolicy",
			constant.ReasonAdmissionPolicyInvalid},
		{"validatingadmissionpolicybinding",
			constant.ReasonAdmissionBindingInvalid},
		{"mutatingadmissionpolicy",
			constant.ReasonMutatingAdmissionPolicyInvalid},
		{"resourceclaim", constant.ReasonResourceClaimFailure},
		{"gateway", constant.ReasonCustomResourceFailure},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			assert.Equal(t, tt.want, customReason(tt.kind))
		})
	}
}
