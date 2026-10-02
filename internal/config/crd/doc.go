// Package crd watches KwatchConfig resources, including ones installed
// after startup, and reports configuration changes. It owns the watcher's
// late-install and restart lifecycle; informer mechanics come from
// inventory/kube/dynamicwatch.
package crd
