# ADR 0008: Application-owned lifecycle supervision

## Status

Accepted

## Decision

The application owns long-running component goroutines, cancellation, failure
reporting, and bounded shutdown. Health owns only its HTTP listener. Delivery
workers expose explicit completion and observe cancellation during pacing and
retry waits.

Compatibility `Start` methods may remain for embedded callers, but application
composition uses `Open`, `Serve`, and `Stop` or the component supervisor
directly.

## Consequences

Required failures can terminate the process deliberately, optional failures
degrade health without failing readiness, and shutdown ordering is testable.
