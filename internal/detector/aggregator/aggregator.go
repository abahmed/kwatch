package aggregator

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
	"github.com/abahmed/kwatch/internal/detector/store"
)

type Aggregator struct {
	aggregation *store.Aggregation
	store       *store.Store
}

func NewAggregator(store *store.Store) *Aggregator {
	return &Aggregator{
		aggregation: store.NewAggregation(10 * time.Minute),
		store:       store,
	}
}

func (a *Aggregator) Name() string {
	return "Aggregator"
}

func (a *Aggregator) Process(input *detector.Input) *detector.Event {
	if input.Pod == nil {
		return nil
	}

	shouldAggregate := a.aggregation.ShouldAggregate(input)
	if !shouldAggregate {
		return nil
	}

	summary := a.aggregation.GetSummary(a.getPodKey(input))
	if summary == "" {
		summary = fmt.Sprintf("Multiple failures detected for pod %s/%s",
			input.Pod.Namespace, input.Pod.Name)
	}

	return &detector.Event{
		Type:      "summary",
		Name:      input.Pod.Name,
		Container: getContainerName(input),
		Namespace: input.Pod.Namespace,
		Node:      input.Pod.Spec.NodeName,
		Reason:    input.Reason,
		Message:   summary,
	}
}

func (a *Aggregator) ShouldAggregate(input *detector.Input) bool {
	return a.aggregation.ShouldAggregate(input)
}

func (a *Aggregator) getPodKey(input *detector.Input) string {
	container := ""
	if input.Container != nil {
		container = input.Container.Name
	}
	return input.Pod.Namespace + "/" + input.Pod.Name + "/" + container
}

func getContainerName(input *detector.Input) string {
	if input.Container == nil {
		return ""
	}
	return input.Container.Name
}

func (a *Aggregator) GetStats() AggregatorStats {
	aggStats := a.aggregation.GetStats()
	return AggregatorStats{
		TotalAggregated: aggStats.TotalAggregated,
		Window:          aggStats.Window,
	}
}

type AggregatorStats struct {
	TotalAggregated int
	Window          time.Duration
}
