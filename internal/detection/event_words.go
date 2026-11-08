package detection

// eventWords are the observed events of a finding's mode, in the words
// of a timeline: what was seen, never why.
var eventWords = map[Mode]string{
	ModeCrashLoop:      "crash",
	ModeRestarting:     "crash",
	ModeOOMKilled:      "out of memory",
	ModeEvicted:        "evicted",
	ModeDiskPressure:   "disk pressure",
	ModeMemoryPressure: "memory pressure",
	ModePIDPressure:    "pid pressure",
	ModeNotReady:       "not ready",
	ModeUnavailable:    "unavailable",
	ModeNoEndpoints:    "no ready endpoints",
	ModeImagePull:      "image pull failing",
	ModeUnschedulable:  "unschedulable",
}

// EventWord is the timeline word of a mode ("crash", "evicted"), false
// for a mode that has none.
func EventWord(mode Mode) (string, bool) {
	word, ok := eventWords[mode]
	return word, ok
}
