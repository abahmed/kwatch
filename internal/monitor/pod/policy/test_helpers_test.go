package policy

import "time"

func testNow() time.Time { return time.Now() }

func suppresses(detector Detector, ctx *Context) bool {
	return detector.Detect(ctx) == DecisionSuppress
}
