package app

import (
	"context"
)

// boundedShutdownContext creates an application-owned shutdown deadline. It
// ignores active-component cancellation while retaining context values, so
// final cleanup can complete after cancellation without becoming unbounded.
func boundedShutdownContext(
	parent context.Context,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(
		context.WithoutCancel(parent), componentShutdownTimeout,
	)
}
