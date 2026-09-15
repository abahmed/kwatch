package message

import (
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// NewReportBuilder preserves the historical function-clock constructor for
// standalone renderers. Delivery composition uses NewReportBuilderWithClock.
func NewReportBuilder(
	cluster string,
	clocks ...func() time.Time,
) *ReportBuilder {
	return NewReportBuilderWithClock(
		cluster, clock.Func(clock.From(clocks)),
	)
}
