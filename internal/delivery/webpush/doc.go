// Package webpush builds Web Push messages for browsers (RFC 8030),
// so the dashboard can notify people whose kwatch tab is closed.
//
// In: one browser's subscription (its push service address and keys),
// a payload and kwatch's VAPID key. Out: a transport.Request that
// carries the payload encrypted for that browser alone (RFC 8291,
// aes128gcm) and signed with the VAPID key (RFC 8292). Only the push
// services of the major browsers are accepted as addresses, so a
// subscription can never make kwatch call anywhere else.
package webpush
