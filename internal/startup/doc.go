// Package startup owns restart state and application startup reporting.
// Kubernetes handlers only provide baseline counts; this package turns those
// counts into an optional startup summary for the application to deliver.
package startup
