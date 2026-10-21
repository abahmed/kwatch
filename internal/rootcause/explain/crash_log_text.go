package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// previousLogLine is the first error line of a crashed container's
// previous log, as the crash-log round recorded it on the container.
// A finding quotes it as the container's "error" only while the
// termination message is empty; a message that says nothing ("Error",
// "exit status 1") hides it. The solver still reads it, so a crash whose
// log names the Service or endpoint it could not reach is explained by
// it. The line is returned only when no finding of the container quotes
// it already.
func (v *view) previousLogLine(id inventory.EntityID) string {
	if id.Kind != kube.KindContainer {
		return ""
	}
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return ""
	}
	line := attrText(e, kube.AttrLastErrorLine)
	if line == "" {
		return ""
	}
	for _, f := range v.s.Findings[id] {
		for _, ev := range f.Evidence {
			if ev.Value == line {
				return ""
			}
		}
	}
	return line
}

// evidenceTexts are the values a finding quotes, then the previous log
// line it does not quote.
func (v *view) evidenceTexts(f detection.Finding) []string {
	out := make([]string, 0, len(f.Evidence)+1)
	for _, e := range f.Evidence {
		out = append(out, e.Value)
	}
	if line := v.previousLogLine(f.Entity); line != "" {
		out = append(out, line)
	}
	return out
}
