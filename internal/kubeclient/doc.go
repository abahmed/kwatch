// Package kubeclient owns Kubernetes access for the application:
// construction of the Kubernetes, dynamic, discovery, REST, HTTP and
// resolver clients, plus small helpers for trimming informer objects,
// panic-safe event handlers and the namespace kwatch runs in. Only the
// composition root constructs clients; other packages receive the narrow
// client they need.
package kubeclient
