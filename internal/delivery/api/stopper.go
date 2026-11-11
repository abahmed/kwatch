package api

// Stopper is implemented by providers that keep work running after a send
// returned, such as a close retried in the background. Delivery calls
// Stop once, after the last send of a shutdown, so that work ends with
// the process instead of with the request that started it.
type Stopper interface {
	Stop()
}
