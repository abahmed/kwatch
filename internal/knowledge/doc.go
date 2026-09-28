// Package knowledge is Kwatch's model of the cluster: entities, the typed
// relations between them, their current attributes, and the history of
// meaningful changes. It is the single source that detectors and the
// reasoning engine read.
//
// The package knows nothing about Kubernetes. Sources translate objects
// into Facts; the Model applies them and maintains indexes so that every
// lookup is proportional to the answer, never to the cluster size.
package knowledge
