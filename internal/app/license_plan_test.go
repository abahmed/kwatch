package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/license"
)

func TestPlanLabelShowsEveryPlanAsPro(t *testing.T) {
	assert.Equal(t, "Pro", planLabel(license.PlanTrial))
	assert.Equal(t, "Pro", planLabel(license.PlanPro))
	assert.Empty(t, planLabel(""))
}
