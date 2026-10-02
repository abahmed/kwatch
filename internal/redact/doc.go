// Package redact removes secrets from text before kwatch keeps or sends it.
// In: raw log lines, condition messages and rendered evidence. Out: the same
// text with credentials, and optionally private addresses, replaced by
// placeholders. It is a leaf package used at ingest and by the e2e sanitizer.
package redact
