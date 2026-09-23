package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/insight"
)

type failingFeedbackSaver struct{}

func (failingFeedbackSaver) SaveRCAFeedback(
	context.Context, []insight.RCARecord,
) error {
	return errors.New("feedback unavailable")
}

func TestFeedbackSaverFinalFailureIsReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []insight.RCARecord, 1)
	ch <- []insight.RCARecord{{}}
	cancel()
	var reported error
	startFeedbackSaver(
		ctx, failingFeedbackSaver{}, ch,
		func(err error) { reported = err },
		func() bool { return true }, time.Now, nil,
	)
	if reported == nil {
		t.Fatal("feedback persistence failure was not reported")
	}
}
