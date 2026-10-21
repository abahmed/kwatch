package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func customResource(kind inventory.Kind, custom bool,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kind, "default", "thing")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom: inventory.Bool(custom),
	})
	return m, id
}

func TestCustomFailingCondition(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Ready", "False", "Broken", "boom", t0)
	setCondition(m, id, "Degraded", "True", "", "", t0.Add(time.Minute))

	early := evaluate(Custom{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	got := evaluate(Custom{}, m, t0.Add(2*time.Minute), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.CustomResourceFailure, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, "Reports Degraded=True, Ready=False (Broken)",
		got[0].Summary)
}

func TestCustomNotReadyWithMessage(t *testing.T) {
	m := newTestModel()
	id := newID("widget", "default", "thing")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom:  inventory.Bool(true),
		kube.AttrReady:   inventory.Bool(false),
		kube.AttrMessage: inventory.Text("waiting"),
	})
	got := evaluate(Custom{}, m, t0.Add(time.Hour), id, nil).Findings
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "not ready")
	assert.Equal(t, "waiting", got[0].Evidence[0].Value)
}

func TestCustomQuiet(t *testing.T) {
	now := t0.Add(time.Hour)
	m, id := customResource("widget", false)
	setCondition(m, id, "Ready", "False", "", "", t0)
	assert.Empty(t, evaluate(Custom{}, m, now, id, nil).Findings,
		"not a custom resource")

	m, id = customResource("widget", true)
	setCondition(m, id, "Ready", "True", "", "", t0)
	assert.Empty(t, evaluate(Custom{}, m, now, id, nil).Findings)

	assert.Equal(t, "custom-resource", Custom{}.Name())
	assert.Equal(t, []inventory.Kind{detection.AnyKind}, Custom{}.Kinds())
}

func TestCustomReasonByKind(t *testing.T) {
	const pf = reasons.APIPriorityAndFairnessFailure
	tests := []struct {
		kind inventory.Kind
		want string
	}{
		{"apiservice", reasons.APIServiceFailure},
		{"volumesnapshot", reasons.VolumeSnapshotFailure},
		{"certificatesigningrequest",
			reasons.CertificateSigningRequestFailure},
		{"flowschema", pf},
		{"prioritylevelconfiguration", pf},
		{"validatingadmissionpolicy",
			reasons.AdmissionPolicyInvalid},
		{"validatingadmissionpolicybinding",
			reasons.AdmissionBindingInvalid},
		{"mutatingadmissionpolicybinding",
			reasons.AdmissionBindingInvalid},
		{"mutatingadmissionpolicy",
			reasons.MutatingAdmissionPolicyInvalid},
		{"resourceclaim", reasons.ResourceClaimFailure},
		{"gateway", reasons.CustomResourceFailure},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			assert.Equal(t, tt.want, customReason(tt.kind))
		})
	}
}

// A failing condition's message is what the controller says is wrong; it
// is quoted from the earliest failing condition, whatever its type.
func TestCustomQuotesTheMessageOfTheEarliestFailingCondition(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Degraded", "True", "PodsCrashing",
		"2 of 3 pods are crash looping", t0.Add(time.Minute))
	setCondition(m, id, "Ready", "False", "Failed",
		"Issuing certificate: order failed: rate limited", t0)

	got := evaluate(Custom{}, m, t0.Add(time.Hour), id, nil).Findings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "message",
		Value: "Issuing certificate: order failed: rate limited"})
}

// Conditions that are bad news by their own status are reported once
// they last, and their message is quoted. Ready=Unknown waits longer
// (custom_unknown_ready_test.go).
func TestCustomReportsFailingAndSucceededConditions(t *testing.T) {
	cases := []struct{ conditionType, status, summary string }{
		{"Failing", "True", "Reports Failing=True (Why)"},
		{"Succeeded", "False", "Reports Succeeded=False (Why)"},
	}
	for _, tc := range cases {
		t.Run(tc.conditionType+"="+tc.status, func(t *testing.T) {
			m, id := customResource("widget", true)
			setCondition(m, id, tc.conditionType, tc.status, "Why",
				"what the controller said", t0)

			early := evaluate(Custom{}, m, t0.Add(time.Minute), id, nil)
			got := evaluate(Custom{}, m, t0.Add(DefaultCustomFailing), id,
				nil).Findings

			assert.Empty(t, early.Findings)
			require.Len(t, got, 1)
			assert.Equal(t, tc.summary, got[0].Summary)
			assert.Contains(t, got[0].Evidence, detection.Evidence{
				Label: "message", Value: "what the controller said"})
		})
	}
}
