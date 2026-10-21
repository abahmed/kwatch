package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogFormatChangesKlogOnlyWhenTheFormatChanges(t *testing.T) {
	var calls []string
	l := &logFormat{set: func(f string) { calls = append(calls, f) }}

	l.apply("json")
	l.apply(" JSON ")
	l.apply("text")
	l.apply("text")

	assert.Equal(t, []string{"json", "text"}, calls)
}
