package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"k8s.io/klog/v2"
)

// logFormatJSON is the app.logFormatter value that switches klog to one
// JSON object per line.
const logFormatJSON = "json"

// jsonLogVerbosity lets every V level through the JSON sink; klog's own
// -v flag still decides which V levels are logged at all.
const jsonLogVerbosity = 10

// applyLogFormat applies app.logFormatter: "json" writes structured JSON
// lines to stderr, anything else keeps klog's text format. It runs again
// after the KwatchConfig overlay, so an overlay can also switch back to text.
func applyLogFormat(format string) {
	if strings.EqualFold(strings.TrimSpace(format), logFormatJSON) {
		klog.SetLogger(newJSONLogger(os.Stderr))
		return
	}
	klog.ClearLogger()
}

// newJSONLogger writes each log entry as one JSON object to out.
func newJSONLogger(out io.Writer) logr.Logger {
	return funcr.NewJSON(func(obj string) {
		_, _ = fmt.Fprintln(out, obj)
	}, funcr.Options{Verbosity: jsonLogVerbosity, LogTimestamp: true})
}
