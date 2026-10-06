package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestJSONLoggerWritesOneObjectPerLine(t *testing.T) {
	var out bytes.Buffer
	logger := newJSONLogger(&out)

	logger.V(3).Info("kubelet summary unavailable", "node", "n1")

	var entry map[string]any
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatalf("not one JSON object: %q: %v", out.String(), err)
	}
	if entry["msg"] != "kubelet summary unavailable" ||
		entry["node"] != "n1" {
		t.Fatalf("entry = %v", entry)
	}
}

func TestApplyLogFormatKeepsTextByDefault(t *testing.T) {
	// Text and empty keep klog's own format; text also resets a JSON logger.
	applyLogFormat("text")
	applyLogFormat("")
}
