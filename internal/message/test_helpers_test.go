package message

import "github.com/abahmed/kwatch/internal/clock"

func newTestReportBuilder(cluster string) *ReportBuilder {
	return NewReportBuilderWithClock(cluster, clock.RealClock{})
}
