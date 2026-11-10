package app

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyStop registers the process's stop signals once, for every
// generation of the run, so there is no moment without a handler: a
// signal that arrives while one generation ends or the next is built is
// remembered, not lost and not left to kill the process outright. The
// returned channel closes at the first signal and stays closed; release
// unregisters the handler.
func notifyStop() (stop <-chan struct{}, release func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	stopped := make(chan struct{})
	quit := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case <-signals:
			close(stopped)
		case <-quit:
		}
	}()
	return stopped, func() {
		signal.Stop(signals)
		close(quit)
		<-exited
	}
}

// stopRequested reports whether a stop signal has arrived. A nil channel
// never has one.
func stopRequested(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}
