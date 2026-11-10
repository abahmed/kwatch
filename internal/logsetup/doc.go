// Package logsetup decides where kwatch's klog output goes and in what
// shape. Container log collectors mark every stderr line as an error, so
// ordinary lines go to stdout and only errors go to stderr. An optional
// JSON format writes one object per line with a "level" field.
package logsetup
