# ADR 0008: Application-owned lifecycle supervision

## Status

Accepted

## Decision

The application owns long-running component goroutines, cancellation, failure
reporting, and bounded shutdown. Health owns only its HTTP listener. Delivery
workers expose explicit completion and observe cancellation during pacing and
retry waits.

Application composition uses `Open`, `Serve`, and `Stop` or the component
supervisor directly; no transitional lifecycle wrapper is retained.

Required components expose lifecycle progress to the supervisor. A component
that exceeds its bounded startup or stall deadline is treated as failed:
readiness is removed, the active generation is canceled, and Lease leadership
is released so Kubernetes can restart or replace the Pod. Optional components
retry with bounded backoff and clear their degraded state after a healthy
restart.

## Consequences

Required failures can terminate the process deliberately, optional failures
degrade health without failing readiness, and shutdown ordering is testable.
