// Package dashboard serves the read-only incident dashboard API on a
// separate listener, protected by a bearer token. It is Pro only and off
// by default. The API exposes what is wrong right now (active incidents),
// resolved incidents, and per-incident detail, plus kwatch's own health.
// Browsers that turn notifications on are sent each change by Web Push
// (push_*.go), so they hear of it with the dashboard closed.
package dashboard
