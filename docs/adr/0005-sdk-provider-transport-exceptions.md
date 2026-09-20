# ADR 0005: Bounded SDK transport exceptions

## Status

Accepted

## Context

Kwatch providers use `internal/delivery/transport` for ordinary HTTP
requests, response handling, retry classification, and redaction. Slack's
token API and Discord's webhook API also have SDK clients that own provider
specific request encoding and protocol behavior. Replacing those SDK calls
with hand-written requests would duplicate protocol logic in provider code.

## Decision

Slack token API calls and Discord webhook execution are the only approved SDK
transport exceptions. They must use the application-provided HTTP client and
caller context. The SDK adapters may translate SDK-specific errors into
Kwatch's shared error categories, but they must not implement their own retry
loops, retry-after policy, or generic HTTP status classification.

Slack webhook verification and all other raw HTTP operations use shared
delivery transport. Providers must never create a process-wide client, log
complete payloads or credentials, or hide a client in a context value.

Adding another SDK exception requires a new ADR, an architecture-check
allowlist entry, injected client tests, cancellation tests, and a documented
error-classification boundary.

## Consequences

The two SDK adapters remain easy to identify and audit, while common delivery
behavior stays centralized. The architecture check rejects new provider
`net/http` imports unless they are one of these explicitly approved files.
