package api

// ThreadSwitch is implemented by providers that keep one chat thread per
// incident. DisableThreads makes the provider send every message of an
// incident as a new top-level message, the way a provider without
// threads does. It is called once, before the provider sends anything.
type ThreadSwitch interface {
	DisableThreads()
}
