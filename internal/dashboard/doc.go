// Package dashboard serves the read-only incident dashboard API on a
// separate listener, protected by a bearer token. It is Pro only and off
// by default. The API exposes what is wrong right now (active incidents),
// resolved incidents, and per-incident detail, plus kwatch's own health.
package dashboard
