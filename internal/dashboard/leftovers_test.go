package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// leftoverIssue is an issue that groups two objects a removed operator
// left behind, rooted at its gone Service.
func leftoverIssue() incident.Incident {
	backend := inventory.CoreID("service", "policy-system", "policy-svc")
	p := testIncident("left", "policy-system", -time.Hour)
	p.Root, p.Tier = backend, incident.Digest
	p.Members = map[detection.Key]detection.Finding{}
	for _, f := range []detection.Finding{
		{Entity: inventory.CoreID("validatingwebhookconfiguration", "",
			"policy-hook"), Reason: "WebhookBackendNotFound",
			Summary: "Admission webhook calls Service policy-svc, " +
				"which does not exist"},
		{Entity: inventory.CoreID("secret", "policy-system",
			"policy-tls"), Reason: "TLSCertExpired",
			Summary: "TLS certificate expired on 2025-01-01"},
	} {
		f.Leftover = backend
		p.Members[f.Key()] = f
	}
	return p
}

func TestLeftoverIssueListsItsObjects(t *testing.T) {
	s := newTestServer(leftoverIssue(), testIncident("other", "a", 0))

	w := get(t, s, "/api/incidents/left")
	require.Equal(t, http.StatusOK, w.Code)
	var got IncidentDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 2, got.LeftoverObjects)
	require.Len(t, got.Leftovers, 2)
	assert.Equal(t, LeftoverObject{Kind: "Secret",
		Namespace: "policy-system", Name: "policy-tls",
		Reason: "TLSCertExpired",
		What:   "TLS certificate expired on 2025-01-01"}, got.Leftovers[0])

	assert.Equal(t, 2, toListItem(leftoverIssue()).LeftoverObjects)
	assert.Zero(t, toListItem(testIncident("other", "a", 0)).
		LeftoverObjects)
	assert.Nil(t, toLeftovers(testIncident("other", "a", 0)))
}

// oldProblemReader is a reader that also counts old problems.
type oldProblemReader struct {
	fakeIncidents
	old []detection.Finding
}

func (r *oldProblemReader) OldProblems() []detection.Finding {
	return r.old
}

func TestOverviewCountsOldProblemsNotShown(t *testing.T) {
	s := newTestServer()
	since := testEpoch.Add(-637 * 24 * time.Hour)
	s.SetIncidents(&oldProblemReader{old: []detection.Finding{{
		Entity: inventory.CoreID("job", "shop", "report"),
		Reason: "JobFailed", Summary: "The job failed", Since: since,
	}}})

	w := getPart(t, s, "/api/overview", "oldProblems")
	require.Equal(t, http.StatusOK, w.Code)
	var got OldProblems
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, OldProblems{Count: 1, Items: []OldProblem{{
		Kind: "Job", Namespace: "shop", Name: "report",
		Reason: "JobFailed", What: "The job failed", Since: since,
		AgeDays: 637}}}, got)

	plain := newTestServer()
	assert.Nil(t, plain.oldProblems(testEpoch),
		"a reader that cannot say says nothing")
}

// An issue lowered for its age says why on its page.
func TestChronicIssueSaysWhyItIsLow(t *testing.T) {
	p := testIncident("old", "a", -time.Hour)
	p.Tier, p.Journal.Level = incident.Digest, "chronic:800"
	s := newTestServer(p)

	w := get(t, s, "/api/incidents/old")
	require.Equal(t, http.StatusOK, w.Code)
	var got IncidentDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotNil(t, got.Why)
	assert.Contains(t, got.Why.AlertLevelReason, "Lowered to Low: "+
		"already failing for about 2 years when kwatch first saw it")
}

// An object with two failures is one row that names both, so the rows
// count objects as leftoverObjects does.
func TestLeftoverObjectWithTwoFailuresIsOneRow(t *testing.T) {
	p := leftoverIssue()
	extra := detection.Finding{Entity: inventory.CoreID("secret",
		"policy-system", "policy-tls"), Reason: "SecretUnused",
		Summary: "Nothing uses this Secret", Leftover: p.Root}
	p.Members[extra.Key()] = extra

	rows := toLeftovers(p)

	require.Len(t, rows, p.LeftoverObjects())
	assert.Equal(t, LeftoverObject{Kind: "Secret",
		Namespace: "policy-system", Name: "policy-tls",
		Reason: "SecretUnused, TLSCertExpired",
		What: "Nothing uses this Secret; " +
			"TLS certificate expired on 2025-01-01"}, rows[0])
}

// The cap counts objects: 100 objects are listed even when some have
// two failures, and the cut is leftoverObjects minus the rows.
func TestLeftoverCapCountsObjects(t *testing.T) {
	p := leftoverIssue()
	p.Members = map[detection.Key]detection.Finding{}
	for i := 0; i < maxLeftovers+5; i++ {
		for _, reason := range []string{"TLSCertExpired", "SecretUnused"} {
			f := detection.Finding{Entity: inventory.CoreID("secret",
				"shop", fmt.Sprintf("web-tls-%03d", i)), Reason: reason,
				Summary: reason, Leftover: p.Root}
			p.Members[f.Key()] = f
		}
	}

	rows := toLeftovers(p)

	require.Len(t, rows, maxLeftovers)
	assert.Equal(t, maxLeftovers+5, p.LeftoverObjects())
	assert.Equal(t, "web-tls-099", rows[maxLeftovers-1].Name)
	assert.Equal(t, "SecretUnused, TLSCertExpired",
		rows[maxLeftovers-1].Reason)
}

// oldDaysReader also says how many days make a problem old.
type oldDaysReader struct {
	oldProblemReader
	days int
}

func (r *oldDaysReader) OldProblemDays() int { return r.days }

// The overview carries the configured age of an old problem, so the
// page does not have to assume 30 days.
func TestOverviewSaysHowManyDaysMakeAProblemOld(t *testing.T) {
	s := newTestServer()
	s.SetIncidents(&oldDaysReader{days: 90})
	got := s.oldProblems(testEpoch)
	require.NotNil(t, got)
	assert.Equal(t, 90, got.Days)

	w := getPart(t, s, "/api/overview", "oldProblems")
	assert.Contains(t, w.Body.String(), `"days":90`)

	s.SetIncidents(&oldProblemReader{})
	w = getPart(t, s, "/api/overview", "oldProblems")
	assert.NotContains(t, w.Body.String(), `"days"`)
}
