package app

import "sync/atomic"

// threadWake connects delivery, which is built before the Lease is held, to
// the thread saver of the current leader session. Without an attached saver
// (standby replica, or between sessions) a notification is a no-op.
type threadWake struct {
	target atomic.Pointer[threadSaver]
}

// Notify is the delivery hook. It never blocks.
func (w *threadWake) Notify() {
	if w == nil {
		return
	}
	if saver := w.target.Load(); saver != nil {
		saver.Wake()
	}
}

func (w *threadWake) attach(saver *threadSaver) {
	if w != nil {
		w.target.Store(saver)
	}
}

func (w *threadWake) detach() {
	if w != nil {
		w.target.Store(nil)
	}
}
