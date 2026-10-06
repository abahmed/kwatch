package kube

import "k8s.io/client-go/tools/cache"

// testDigester keys the digests of tests that build a transform without
// a stored key.
var testDigester = NewDigester(nil)

// testTransform is the informer transform the tests apply to objects.
var testTransform cache.TransformFunc = newTransform(testDigester)
