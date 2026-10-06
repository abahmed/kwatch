package clickup

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func clickupWith(
	t *testing.T, config map[string]interface{},
	reply func(http.ResponseWriter, providertest.Request),
) (*Clickup, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = reply
	base := map[string]interface{}{"token": "t", "listId": "9"}
	for k, v := range config {
		base[k] = v
	}
	c := NewClickup(base, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.api = rec.URL()
	return c, rec
}

func TestCloseStatusMovesTheTask(t *testing.T) {
	c, rec := clickupWith(t, map[string]interface{}{
		"closeStatus": "complete"}, nil)
	require.NoError(t, c.Close(context.Background(), "t1", "fixed"))

	requests := rec.Requests()
	require.Len(t, requests, 2)
	assert.Equal(t, "/task/t1/comment", requests[0].Path)
	assert.Equal(t, "PUT", requests[1].Method)
	assert.Equal(t, "/task/t1", requests[1].Path)
	assert.Equal(t, "complete", requests[1].JSON(t)["status"])
}

func TestWithoutCloseStatusOnlyComments(t *testing.T) {
	c, rec := clickupWith(t, nil, nil)
	require.NoError(t, c.Close(context.Background(), "t1", "fixed"))
	assert.Len(t, rec.Requests(), 1)
}

func TestUnknownStatusIsLoggedNotRetried(t *testing.T) {
	c, _ := clickupWith(t, map[string]interface{}{
		"closeStatus": "nope"},
		func(w http.ResponseWriter, r providertest.Request) {
			if r.Method == "PUT" {
				w.WriteHeader(http.StatusBadRequest)
			}
		})
	assert.NoError(t, c.Close(context.Background(), "t1", "fixed"))
}

func TestReopenStatusMovesTheTaskBack(t *testing.T) {
	c, rec := clickupWith(t, map[string]interface{}{
		"closeStatus": "complete", "reopenStatus": "to do"}, nil)
	require.True(t, c.CanReopen())
	require.NoError(t, c.Reopen(context.Background(), "t1"))
	last := rec.Last(t)
	assert.Equal(t, "PUT", last.Method)
	assert.Equal(t, "to do", last.JSON(t)["status"])
}

func TestReopenNeedsBothStatuses(t *testing.T) {
	c, _ := clickupWith(t, map[string]interface{}{
		"closeStatus": "complete"}, nil)
	assert.False(t, c.CanReopen())
	c, _ = clickupWith(t, map[string]interface{}{
		"reopenStatus": "to do"}, nil)
	assert.False(t, c.CanReopen())
}

func TestClosesIssuesOnlyWithACloseStatus(t *testing.T) {
	without, _ := clickupWith(t, nil, nil)
	assert.False(t, without.ClosesIssues())
	with, _ := clickupWith(t, map[string]interface{}{
		"closeStatus": "complete"}, nil)
	assert.True(t, with.ClosesIssues())
}
