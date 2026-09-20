// Package transport is the shared outbound HTTP boundary for alert delivery.
//
// Provider adapters use this package for request creation, response handling,
// status classification, cancellation, and safe response summaries. Retry
// policy stays in the delivery manager so provider packages remain payload
// adapters rather than independent HTTP clients.
package transport
