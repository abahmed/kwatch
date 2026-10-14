package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withReason runs fn with the table extended by one entry, the way a new
// reason is added: one entry, nothing else.
func withReason(t *testing.T, info ReasonInfo, fn func()) {
	t.Helper()
	saved, savedModes := reasonTable, modeByReason
	defer func() { reasonTable, modeByReason = saved, savedModes }()
	reasonTable = buildReasonTable(
		podReasons, nodeReasons, workloadReasons, serviceReasons,
		eventOnlyReasons, upstreamReasons, admissionReasons,
		[]ReasonInfo{info},
	)
	modeByReason = modesOf(reasonTable)
	fn()
}

func TestOneEntryAddsAReasonEverywhere(t *testing.T) {
	info := reason("DummyReason", Mode("Dummy")).
		waiting(Warning).event(Info).digest().causeOnly().shown().
		label("is a dummy")
	assert.Equal(t, Mode("DummyReason"), ModeFor("DummyReason"))
	withReason(t, info, func() {
		assert.Equal(t, Mode("Dummy"), ModeFor("DummyReason"))
		assert.Equal(t, Mode("Dummy"), Classify(Finding{
			Reason: "DummyReason", Severity: Warning,
		}).Mode)
		w, ok := WaitingSeverity("DummyReason")
		assert.True(t, ok)
		assert.Equal(t, Warning, w)
		e, ok := EventSeverity("DummyReason")
		assert.True(t, ok)
		assert.Equal(t, Info, e)
		assert.True(t, DigestOnly("DummyReason"))
		assert.True(t, CauseOnly("DummyReason"))
		assert.True(t, ShownByState("DummyReason"))
		assert.Equal(t, "is a dummy", ReasonLabel("DummyReason"))
		assert.True(t, KnownMode(Mode("Dummy")))
	})
	_, ok := WaitingSeverity("DummyReason")
	assert.False(t, ok, "the table is restored")
}

func TestReasonTableRejectsADuplicate(t *testing.T) {
	assert.Panics(t, func() {
		buildReasonTable(
			[]ReasonInfo{reason("X", ModeError)},
			[]ReasonInfo{reason("X", ModeFailed)},
		)
	})
}

func TestUnlistedReasonHasNoAttributes(t *testing.T) {
	_, waiting := WaitingSeverity("Nope")
	_, event := EventSeverity("Nope")
	assert.False(t, waiting || event)
	assert.False(t, DigestOnly("Nope") || CauseOnly("Nope"))
	assert.False(t, ShownByState("Nope"))
	require.Empty(t, ReasonLabel("Nope"))
}

// A Warning event is either a failure by itself or already shown by the
// object state, never both: that would report one failure twice.
func TestEventIsNeverBothFailureAndShown(t *testing.T) {
	for name, info := range reasonTable {
		assert.False(t, info.Event != 0 && info.ShownByState, name)
	}
}
