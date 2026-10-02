package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidSeverity(t *testing.T) {
	valid := []string{
		"critical", "high", "medium", "warning", "normal",
		"High", "CRITICAL", " Warning ",
	}
	for _, value := range valid {
		assert.True(t, IsValidSeverity(value),
			"expected %q to be valid", value)
	}
	invalid := []string{"severe", "urgent", "", "critical!", "p0"}
	for _, value := range invalid {
		assert.False(t, IsValidSeverity(value),
			"expected %q to be invalid", value)
	}
}

func TestNormalizeSeverity(t *testing.T) {
	assert.Equal(t, SeverityHigh, NormalizeSeverity("High"))
	assert.Equal(t, SeverityCritical, NormalizeSeverity(" CRITICAL "))
	assert.Equal(t, SeverityNormal, NormalizeSeverity("normal"))
}
