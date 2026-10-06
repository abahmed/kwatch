package jira

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const transitionsAnswer = `{"transitions":[
 {"id":"11","name":"Start","to":{"name":"In Progress"}},
 {"id":"31","name":"Close it","to":{"name":"Done"}}]}`

func closingJira(
	t *testing.T, config map[string]interface{},
	reply func(http.ResponseWriter, providertest.Request),
) (*Jira, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = reply
	base := map[string]interface{}{
		"url": rec.URL(), "user": "u", "apiToken": "s", "projectKey": "OPS",
	}
	for k, v := range config {
		base[k] = v
	}
	j := NewJira(base, "dev", rec.Dependencies())
	require.NotNil(t, j)
	return j, rec
}

func answerTransitions(w http.ResponseWriter, r providertest.Request) {
	if r.Method == "GET" {
		_, _ = w.Write([]byte(transitionsAnswer))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func TestResolveAppliesTheDoneTransition(t *testing.T) {
	j, rec := closingJira(t, map[string]interface{}{
		"closeTransition": "Done"}, answerTransitions)
	require.NoError(t, j.Close(context.Background(), "OPS-9", "fixed"))

	requests := rec.Requests()
	require.Len(t, requests, 3)
	assert.Equal(t, "/rest/api/2/issue/OPS-9/comment", requests[0].Path)
	assert.Equal(t, "GET", requests[1].Method)
	assert.Equal(t, "/rest/api/2/issue/OPS-9/transitions", requests[2].Path)
	body := requests[2].JSON(t)
	assert.Equal(t, "31", body["transition"].(map[string]any)["id"])
}

func TestConfiguredTransitionNameIsUsed(t *testing.T) {
	j, rec := closingJira(t, map[string]interface{}{
		"closeTransition": "start"}, answerTransitions)
	require.NoError(t, j.Close(context.Background(), "OPS-9", "fixed"))
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "11", body["transition"].(map[string]any)["id"])
}

func TestMissingTransitionLeavesTheIssueAlone(t *testing.T) {
	j, rec := closingJira(t, map[string]interface{}{
		"closeTransition": "Nonexistent"}, answerTransitions)
	require.NoError(t, j.Close(context.Background(), "OPS-9", "fixed"))
	assert.Len(t, rec.Requests(), 2, "comment and the transitions lookup")
}

func TestRefusedTransitionIsNotRetried(t *testing.T) {
	j, _ := closingJira(t, map[string]interface{}{
		"closeTransition": "Done"},
		func(w http.ResponseWriter, r providertest.Request) {
			if r.Method == "GET" {
				_, _ = w.Write([]byte(transitionsAnswer))
				return
			}
			if r.Path != "/rest/api/2/issue/OPS-9/comment" {
				w.WriteHeader(http.StatusBadRequest)
			}
		})
	assert.NoError(t, j.Close(context.Background(), "OPS-9", "fixed"))
}

func TestTransientTransitionFailureIsRetried(t *testing.T) {
	j, _ := closingJira(t, map[string]interface{}{
		"closeTransition": "Done"},
		func(w http.ResponseWriter, r providertest.Request) {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusBadGateway)
			}
		})
	assert.Error(t, j.Close(context.Background(), "OPS-9", "fixed"))
}

func TestEmptyCloseTransitionOnlyComments(t *testing.T) {
	j, rec := closingJira(t, map[string]interface{}{
		"closeTransition": ""}, answerTransitions)
	require.NoError(t, j.Close(context.Background(), "OPS-9", "fixed"))
	assert.Len(t, rec.Requests(), 1)
}

func TestWikiMarkupInTheNoteIsEscaped(t *testing.T) {
	got := escapeWiki("hi [~accountid:1] {code} !http://x/i.png! ok!")
	assert.Equal(t,
		`hi \[~accountid:1] \{code} \!http://x/i.png! ok!`, got)
}

func TestJSONLikeTextIsLeftAlone(t *testing.T) {
	in := `{"a": [1, 2], "b": {"c": "d"}} and [x] done!`
	assert.Equal(t, in, escapeWiki(in))
}

func TestLinksAndColorMacrosAreEscaped(t *testing.T) {
	assert.Equal(t, `\[go|http://x] \{color:red}z\{color}`,
		escapeWiki(`[go|http://x] {color:red}z{color}`))
}

func TestCompleteCodeBlockIsKept(t *testing.T) {
	in := `a {code}[~x] {b}{code} b`
	assert.Equal(t, in, escapeWiki(in))
	assert.Equal(t, `\{noformat} open`, escapeWiki(`{noformat} open`))
}

func TestNoDefaultCloseTransition(t *testing.T) {
	j, rec := closingJira(t, nil, answerTransitions)
	require.NoError(t, j.Close(context.Background(), "OPS-9", "fixed"))
	assert.Len(t, rec.Requests(), 1, "comment only")
	assert.False(t, j.CanReopen())
}

func TestReopenAppliesTheReopenTransition(t *testing.T) {
	j, rec := closingJira(t, map[string]interface{}{
		"closeTransition": "Done", "reopenTransition": "Start"},
		answerTransitions)
	require.True(t, j.CanReopen())
	require.NoError(t, j.Reopen(context.Background(), "OPS-9"))
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "11", body["transition"].(map[string]any)["id"])
}

func TestReopenNeedsBothTransitions(t *testing.T) {
	j, _ := closingJira(t, map[string]interface{}{
		"reopenTransition": "Start"}, answerTransitions)
	assert.False(t, j.CanReopen())
}

func TestClosesIssuesOnlyWithACloseTransition(t *testing.T) {
	without, _ := closingJira(t, nil, answerTransitions)
	assert.False(t, without.ClosesIssues())
	with, _ := closingJira(t, map[string]interface{}{
		"closeTransition": "Done"}, answerTransitions)
	assert.True(t, with.ClosesIssues())
}
