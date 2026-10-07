// Package status builds the read-only "what is wrong right now" view
// that the health server serves on /status, and the upgrade-readiness
// answer the digest repeats when it changes. It only reads state kwatch
// already holds (incidents, active findings, the model, coverage): it
// runs no loop of its own and writes nothing. Text and JSON come from the
// same Report, so the two always agree.
package status
