// Package monitor defines the metadata and extension contract shared by Kwatch
// monitors.
//
// A monitor detects a domain fact. It does not decide whether a notification
// is sent, write persistence, or call a delivery provider. The registry in
// this package is intentionally small: runtime wiring remains explicit in
// internal/app. Family packages such as monitor/pod own typed detection
// policy; there is intentionally no universal runtime monitor interface.
package monitor
