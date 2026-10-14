package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeEvents = `package events

const (
	Failed  = "FailedThing"
	Started = "StartedThing"
)
`

const fakeController = `package ctrl

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/fake/events"
)

const local = "LocalReason"

var errBoom = errors.New("BoomError")

type c struct{ rec recorder }

func (c *c) run(reason string) {
	c.rec.Event(nil, v1.EventTypeWarning, events.Failed, "m")
	c.rec.Eventf(nil, v1.EventTypeNormal, events.Started, "m")
	c.rec.Event(nil, v1.EventTypeWarning, "Literal", "m")
	c.rec.Event(nil, v1.EventTypeWarning, local, "m")
	c.rec.Event(nil, v1.EventTypeWarning, errBoom, "m")
	c.emit(nil, v1.EventTypeNormal, reason, "m")
	c.note("ViaWrapper")
}

func (c *c) note(reason string) {
	c.rec.Event(nil, v1.EventTypeWarning, reason, "m")
}

func (c *c) dynamic(x string) {
	c.rec.Event(nil, v1.EventTypeWarning, x+"y", "m")
}
`

func fakeCorpus(t *testing.T) *corpus {
	t.Helper()
	root := t.TempDir()
	for name, src := range map[string]string{
		"pkg/fake/events/e.go": fakeEvents,
		"pkg/fake/ctrl/c.go":   fakeController,
	} {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(src), 0o600))
	}
	c, err := load(root, []string{"pkg/fake"})
	require.NoError(t, err)
	c.collectCalls(func(string) bool { return false })
	return c
}

func TestCallsResolveConstantsAndWrappers(t *testing.T) {
	c := fakeCorpus(t)
	got := eventReasons(c.calls)
	assert.Equal(t, map[string]bool{
		"FailedThing":  true,
		"StartedThing": false,
		"Literal":      true,
		"LocalReason":  true,
		"BoomError":    true,
		"ViaWrapper":   true,
	}, got)
}

func TestUnresolvedReasonsAreListed(t *testing.T) {
	c := fakeCorpus(t)
	require.Len(t, c.unresolved, 1)
	assert.Contains(t, c.unresolved[0], "pkg/fake/ctrl/c.go:")
}

func TestParameterShadowsPackageConstant(t *testing.T) {
	root := t.TempDir()
	src := `package k

const reason = "PackageReason"

func reject(reason string) {
	rec.Event(nil, v1.EventTypeWarning, reason, "m")
}

func caller() { reject("Real") }
`
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg/k"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg/k/k.go"),
		[]byte(src), 0o600))
	c, err := load(root, []string{"pkg/k"})
	require.NoError(t, err)
	c.collectCalls(func(string) bool { return false })
	assert.Equal(t, map[string]bool{"Real": true}, eventReasons(c.calls))
}

func TestRefsAndLiteralsFailLoudly(t *testing.T) {
	c := fakeCorpus(t)
	v, err := c.fromRef("pkg/fake/events:Failed")
	require.NoError(t, err)
	assert.Equal(t, "FailedThing", v)

	_, err = c.values(group{
		refs: []string{"pkg/fake/events:Gone"},
		lits: []lit{{"pkg/fake/events/e.go", "NotThere"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Gone")
	assert.Contains(t, err.Error(), "NotThere")
}
