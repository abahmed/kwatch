// Package notification defines the provider-neutral Message kwatch
// delivers: the narrative Note and its Short lead, the application
// output and next steps, the deprecated title, explanation lines and
// timeline, plus the severity levels and rendering helpers (chunking, mention
// neutralizing, fallback text) that every provider shares.
// In: Messages written by notification/compose. Out: what every provider
// renders. It is a leaf package, so providers can import it without
// depending on the domain packages that write it.
package notification
