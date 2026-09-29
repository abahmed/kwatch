// Package story writes the message for a problem decision.
//
// A story is composed from the problem's own evidence: what broke, why
// (the proven cause, or an honest "cause unknown"), who is affected, what
// happened in order, and the next step with commands for the real
// objects. Sections without content are omitted. The result is a
// structured notice.Message that every provider renders in its own format.
package story
