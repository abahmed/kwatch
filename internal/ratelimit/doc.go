// Package ratelimit carries a provider's rate-limit answer: the Error that
// wraps a throttled send with its Retry-After delay, and the parser for
// that header. In: an HTTP response or an SDK's rate-limit error. Out: a
// delay that delivery retry honours.
//
// It stays a leaf instead of living in delivery/transport because the
// event leaf and the SDK-backed providers (Slack, Discord) build and read
// these errors; importing transport from event would point a leaf upward.
package ratelimit
