package app

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/klog/v2"
)

// componentSupervisor owns application-launched goroutines and the single
// failure channel observed by the shutdown loop. Domain components keep their
// own run and stop semantics; the application owns their lifetime.
type componentSupervisor struct {
	wg    sync.WaitGroup
	errCh chan error
}

func newComponentSupervisor() *componentSupervisor {
	return &componentSupervisor{errCh: make(chan error, 1)}
}

func (s *componentSupervisor) startOwned(
	ctx context.Context,
	component componentSpec,
) {
	if component.run == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := component.run(ctx); err != nil {
			if component.required {
				s.report(fmt.Errorf("%s: %w", component.name, err))
				return
			}
			if component.onError != nil {
				component.onError(err)
			}
			klog.ErrorS(err, "application component stopped",
				"component", component.name)
		}
	}()
}

func (s *componentSupervisor) startOptional(
	ctx context.Context,
	initialized <-chan struct{},
	component componentSpec,
) {
	if component.run == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if !waitForInitialization(ctx, initialized) {
			return
		}
		klog.InfoS(
			"starting optional component",
			"component", component.name,
			"required", component.required,
		)
		if err := component.run(ctx); err != nil {
			if component.required {
				s.report(fmt.Errorf("%s: %w", component.name, err))
				return
			}
			if component.onError != nil {
				component.onError(err)
			}
			klog.ErrorS(err, "optional component stopped",
				"component", component.name)
		}
	}()
}

func (s *componentSupervisor) report(err error) {
	if err == nil {
		return
	}
	select {
	case s.errCh <- err:
	default:
		klog.ErrorS(err, "application component failure was already reported")
	}
}
