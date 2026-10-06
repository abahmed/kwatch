// Package announce decides which decisions are collected into one
// message instead of sent one by one: the startup summary, the digest of
// low-tier news, the roll-up of announcements made together, and the
// namespace outage. It also keeps the listings those messages made, so
// the first own message of a listed incident introduces it and the
// listing closes once everything it named has resolved.
//
// The pipeline's announcer owns one Collector and calls its steps in a
// fixed order on the decision loop. The Collector does no I/O: it hands
// messages to the sink in Env and never imports the pipeline package.
package announce
