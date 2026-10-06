package kube

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// Bounds on what one change keeps, so a large edit cannot grow the
// history: values are cut, and only the likeliest fields are listed.
const (
	maxFieldValue   = 96
	maxChangeFields = 12
	moreFieldsPath  = "(more fields)"
)

// boundFields ranks the fields of one change, redacts and cuts every
// value, and keeps the likeliest maxChangeFields; one last field counts
// the rest.
func boundFields(fields []inventory.FieldChange) []inventory.FieldChange {
	if len(fields) == 0 {
		return fields
	}
	ranked := inventory.RankFields(fields)
	extra := 0
	if len(ranked) > maxChangeFields {
		extra = len(ranked) - maxChangeFields + 1
		ranked = ranked[:maxChangeFields-1]
	}
	for i := range ranked {
		ranked[i].Before = boundValue(ranked[i].Before)
		ranked[i].After = boundValue(ranked[i].After)
	}
	if extra > 0 {
		ranked = append(ranked, inventory.FieldChange{
			Path: moreFieldsPath, After: strconv.Itoa(extra) + " more"})
	}
	return ranked
}

func boundValue(value string) string {
	return truncateTo(redact.Credentials(value), maxFieldValue)
}
