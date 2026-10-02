// Package compose writes the notification Message for an incident
// decision as a short note an engineer could have written: a lead
// sentence with what broke and why, the one or two facts that prove the
// cause, who is affected, what kwatch could not check, and one command.
//
// Facts become sentences through small writers (one per fact type, see
// noteWriters); the sentences are ordered by part and weight and joined.
// Updates say only what changed; resolves say how long it lasted and,
// when known, who fixed it.
// In: incident.Decisions. Out: notification.Messages for delivery.
package compose
