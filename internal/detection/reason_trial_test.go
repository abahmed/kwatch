package detection

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// graduatedFile lists every reason that interrupts as its entry says.
const graduatedFile = "testdata/graduated_reasons.txt"

// readGraduated reads the reasons of graduatedFile, failing on a reason
// listed twice.
func readGraduated(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(graduatedFile)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	out := map[string]bool{}
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		require.False(t, out[line], "%s is listed twice", line)
		out[line] = true
	}
	require.NoError(t, lines.Err())
	return out
}

// trialStatusErrors names every reason whose trial status is missing or
// contradicts graduated, and every graduated reason the table lost.
func trialStatusErrors(
	table map[string]ReasonInfo, graduated map[string]bool,
) []string {
	var out []string
	for name, info := range table {
		switch {
		case info.Trial && graduated[name]:
			out = append(out, name+": on trial and graduated; "+
				"remove trial() or the "+graduatedFile+" line")
		case !info.Trial && !graduated[name]:
			out = append(out, name+": new reason without trial(); "+
				"mark it trial() or promote it in "+graduatedFile)
		}
	}
	for name := range graduated {
		if _, ok := table[name]; !ok {
			out = append(out, name+": in "+graduatedFile+
				" but not in the reason tables")
		}
	}
	return out
}

// A new reason starts on trial; it interrupts only once it is promoted
// in graduatedFile. A reason with neither is one that skipped its
// trial (rc.1 sent PdbSelectsNothing, years-old states included).
func TestEveryReasonDeclaresItsTrialStatus(t *testing.T) {
	assert.Empty(t, trialStatusErrors(reasonTable, readGraduated(t)))
}

// The check catches each way a reason can get its status wrong.
func TestTrialStatusCheckCatchesMistakes(t *testing.T) {
	table := map[string]ReasonInfo{
		"Old": reason("Old", ModeError),
		"New": reason("New", ModeError),
		"Odd": reason("Odd", ModeError).trial(),
	}
	errs := trialStatusErrors(table,
		map[string]bool{"Old": true, "Odd": true, "Gone": true})
	assert.Len(t, errs, 3)
	joined := strings.Join(errs, "\n")
	for _, name := range []string{"New", "Odd", "Gone"} {
		assert.Contains(t, joined, name+":")
	}
}

// A reason on trial waits for the digest whatever its severity; once
// promoted it is what its entry says.
func TestTrialReasonWaitsForTheDigest(t *testing.T) {
	info := reason("DummyTrial", Mode("Dummy")).trial()
	withReason(t, info, func() {
		assert.True(t, OnTrial("DummyTrial"))
		assert.True(t, DigestOnly("DummyTrial"))
	})
	withReason(t, reason("DummyTrial", Mode("Dummy")), func() {
		assert.False(t, OnTrial("DummyTrial"))
		assert.False(t, DigestOnly("DummyTrial"))
	})
	assert.True(t, OnTrial(reasons.NodePodNetworkConflict))
	assert.True(t, DigestOnly(reasons.NodePodNetworkConflict))
	assert.False(t, OnTrial(reasons.PdbSelectsNothing))
}
