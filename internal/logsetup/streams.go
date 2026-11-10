package logsetup

import (
	"flag"
	"io"
	"os"
	"strings"

	"k8s.io/klog/v2"
)

// Init routes klog's text lines by severity: INFO and WARNING go to
// stdout, ERROR and FATAL to stderr. The line format is klog's own. fs
// is the flag set klog registered its flags on; flags the operator set
// on the command line are overridden because they would send everything
// to stderr again.
func Init(fs *flag.FlagSet) {
	routeText(fs, os.Stdout, os.Stderr)
}

func routeText(fs *flag.FlagSet, stdout, stderr io.Writer) {
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("alsologtostderr", "false")
	// Only fatal exits (with their stack) still use klog's own stderr
	// path; error lines come through the severity writers below.
	_ = fs.Set("stderrthreshold", "FATAL")
	klog.SetOutputBySeverity("INFO", &severityWriter{'I', stdout})
	klog.SetOutputBySeverity("WARNING", &severityWriter{'W', stdout})
	klog.SetOutputBySeverity("ERROR", &severityWriter{'E', stderr})
	klog.SetOutputBySeverity("FATAL", io.Discard)
}

// severityWriter passes only the klog lines of one severity. klog also
// writes a line to the writers of every lower severity, so without the
// filter an error line would appear on stdout as well.
type severityWriter struct {
	letter byte
	out    io.Writer
}

func (w *severityWriter) Write(p []byte) (int, error) {
	if len(p) > 0 && p[0] == w.letter && hasHeader(p) {
		return w.out.Write(p)
	}
	return len(p), nil
}

// hasHeader reports whether p starts like a klog line: a severity letter
// followed by the month and day digits.
func hasHeader(p []byte) bool {
	return len(p) > 4 && p[1] >= '0' && p[1] <= '9'
}

// EnvFormat names the variable that selects the log format.
const EnvFormat = "KWATCH_LOG_FORMAT"

// Format returns the format to use: KWATCH_LOG_FORMAT when set, else the
// configured one.
func Format(configured string) string {
	if v := strings.TrimSpace(os.Getenv(EnvFormat)); v != "" {
		return v
	}
	return configured
}

// Apply is the process start-up entry: it routes text lines by severity
// and switches to JSON when KWATCH_LOG_FORMAT asks for it.
func Apply(fs *flag.FlagSet) {
	Init(fs)
	if Wants(Format("")) {
		UseJSON(os.Stdout, os.Stderr)
	}
}
