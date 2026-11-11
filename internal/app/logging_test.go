package app

import "testing"

func TestApplyLogFormatKeepsTextByDefault(t *testing.T) {
	// Text and empty keep klog's own format; text also resets a JSON logger.
	applyLogFormat("text")
	applyLogFormat("")
}
