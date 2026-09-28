// Package core runs the problem-centric pipeline described in ADR 0010:
//
//	facts → knowledge model → detectors → signal transitions →
//	problems → decisions → stories → sink
//
// One goroutine owns the pipeline, so model updates, signal tracking and
// problem decisions are ordered without further locking. Sources submit
// facts from any goroutine; time-based conditions schedule rechecks.
package core
