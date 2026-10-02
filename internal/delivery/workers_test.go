package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkerSetClosesDoneAfterTheLastWorker(t *testing.T) {
	var workers workerSet
	ctx := workers.launch(context.Background(), 2)
	workers.stuck = true

	assert.True(t, workers.stillStopping())
	assert.False(t, workers.finishOne(), "one worker is still running")
	assert.True(t, workers.finishOne(), "the last worker returned")
	assert.False(t, workers.stillStopping())
	assert.False(t, workers.finishOne(), "no worker is left to finish")
	select {
	case <-workers.done:
	default:
		t.Fatal("done is not closed after the last worker")
	}
	workers.cancel()
	assert.Error(t, ctx.Err(), "cancel ends the workers' context")
}

func TestWorkerSetWithoutWorkersIsDoneAtOnce(t *testing.T) {
	var workers workerSet
	workers.launch(context.Background(), 0)
	select {
	case <-workers.done:
	default:
		t.Fatal("done is not closed for an empty set")
	}
	workers.cancelSend()
	assert.Error(t, workers.sendCtx.Err())
}

func TestDoneSignalClosesOnce(t *testing.T) {
	var done doneSignal
	done.close()
	done.close()
	assert.True(t, done.closed)
	<-done.ch
}
