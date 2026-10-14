// Command upstreamreasons extracts the reason strings Kubernetes emits
// from a pinned kubernetes/kubernetes source tree and writes or checks
// the generated files of internal/detection/upstream. It needs network
// (a shallow git clone) only when asked to fetch; the tests and
// make verify read the committed files.
package main
