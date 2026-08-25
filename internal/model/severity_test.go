package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidSeverity(t *testing.T) {
	for _, valid := range []string{"critical", "high", "medium", "warning", "normal", "High", "CRITICAL", " Warning "} {
		assert.True(t, IsValidSeverity(valid), "expected %q to be valid", valid)
	}
	for _, invalid := range []string{"severe", "urgent", "", "critical!", "p0"} {
		assert.False(t, IsValidSeverity(invalid), "expected %q to be invalid", invalid)
	}
}

func TestNormalizeSeverity(t *testing.T) {
	assert.Equal(t, SeverityHigh, NormalizeSeverity("High"))
	assert.Equal(t, SeverityCritical, NormalizeSeverity(" CRITICAL "))
	assert.Equal(t, SeverityNormal, NormalizeSeverity("normal"))
}
