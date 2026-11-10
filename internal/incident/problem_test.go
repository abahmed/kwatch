package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// SetProblem keeps the bounded and the full title; a record saved
// before the full one existed reads the bounded one as its full one.
func TestSetProblemStoresBothTitles(t *testing.T) {
	m := NewManager(Config{IDNonce: testNonce}, nil)
	m.incidents = map[string]*Incident{"a": {ID: "a"}}

	m.SetProblem("a", "short…", "short and then some")
	m.SetProblem("a", "", "")

	p := *m.incidents["a"]
	assert.Equal(t, "short…", p.Title())
	assert.Equal(t, "short and then some", p.FullTitle())
	assert.Equal(t, "old", Incident{Problem: "old"}.FullTitle())
}
