package incident

import "github.com/abahmed/kwatch/internal/model"

// SkipLogger records lifecycle decisions that intentionally did not create
// an incident. The incident domain needs this small seam, not the concrete
// audit implementation used by the application.
type SkipLogger interface {
	LogSkip(*model.Incident, string)
}
