package reasons

// CrashExit reports a container's last run that ended on its own with
// an error: a non-zero exit code that is neither a stop by signal (137
// is 128 plus SIGKILL, 143 is 128 plus SIGTERM: how the kubelet stops
// a container) nor a memory kill (lastReason OOMKilled). The detectors
// and the dashboard share it so an alert and its page tell a crash
// alike. The codes are not constants here: every constant of this
// package is a reason.
func CrashExit(code int, lastReason string) bool {
	return code != 0 && code != 137 && code != 143 &&
		lastReason != OOMKilled && lastReason != OOMKILLED
}
