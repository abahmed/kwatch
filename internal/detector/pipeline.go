package detector

import (
	"time"

	"github.com/sirupsen/logrus"
)

// PipelineConfig holds pipeline configuration
type PipelineConfig struct {
	Volume          Volume
	Client          interface{}
	Config          interface{}
	DedupWindow     time.Duration
	AggregateWindow time.Duration
}

// pipeline implements the detection pipeline
type pipeline struct {
	config     *PipelineConfig
	predicates []Predicate
	detectors  []Detector
	handlers   []Handler
	dedup      Deduplication
	aggregator Aggregator
}

// NewPipeline creates a new pipeline
func NewPipeline(config *PipelineConfig) *pipeline {
	return &pipeline{
		config:     config,
		predicates: []Predicate{},
		detectors:  []Detector{},
		handlers:   []Handler{},
	}
}

// AddPredicate adds a predicate to the pipeline
func (p *pipeline) AddPredicate(predicate Predicate) {
	p.predicates = append(p.predicates, predicate)
}

// AddDetector adds a detector to the pipeline
func (p *pipeline) AddDetector(detector Detector) {
	p.detectors = append(p.detectors, detector)
}

// AddHandler adds a handler to the pipeline
func (p *pipeline) AddHandler(handler Handler) {
	p.handlers = append(p.handlers, handler)
}

// SetDeduplication sets the deduplication component
func (p *pipeline) SetDeduplication(dedup Deduplication) {
	p.dedup = dedup
}

// SetAggregator sets the aggregator component
func (p *pipeline) SetAggregator(aggregator Aggregator) {
	p.aggregator = aggregator
}

// ProcessPod processes a pod event
func (p *pipeline) ProcessPod(input *Input) *Event {
	// 1. Run predicates (filters)
	for _, pred := range p.predicates {
		if pred.Filter(input) {
			logrus.Debugf("Pod filtered by predicate: %s", pred.Name())
			return nil
		}
	}

	// 2. Run detectors
	issueDetected := false
	for _, detector := range p.detectors {
		if detector.Detect(input) {
			issueDetected = true
			break
		}
	}

	if !issueDetected {
		return nil
	}

	// 3. Run handlers (enrichment)
	for _, handler := range p.handlers {
		if err := handler.Handle(input); err != nil {
			logrus.Warnf("Handler %s failed: %v", handler.Name(), err)
		}
	}

	// 4. Check deduplication
	if p.dedup != nil && !p.dedup.ShouldAlert(input) {
		logrus.Debugf("Pod alert deduplicated: %s/%s", input.Pod.Namespace, input.Pod.Name)
		return nil
	}

	// 5. Process aggregator
	if p.aggregator != nil {
		event := p.aggregator.Process(input)
		if event != nil {
			// Record for deduplication
			if p.dedup != nil {
				p.dedup.Record(input)
			}
			return event
		}
		return nil
	}

	// 6. Build event
	event := p.buildEvent(input)

	// 7. Record for deduplication
	if p.dedup != nil {
		p.dedup.Record(input)
	}

	return event
}

// ProcessNode processes a node event
func (p *pipeline) ProcessNode(input *Input) *Event {
	// Similar to ProcessPod but for node events

	// 1. Run predicates
	for _, pred := range p.predicates {
		if pred.Filter(input) {
			return nil
		}
	}

	// 2. Run detectors
	issueDetected := false
	for _, detector := range p.detectors {
		if detector.Detect(input) {
			issueDetected = true
			break
		}
	}

	if !issueDetected {
		return nil
	}

	// 3. Check deduplication
	if p.dedup != nil && !p.dedup.ShouldAlert(input) {
		return nil
	}

	// 4. Build event
	event := p.buildNodeEvent(input)

	// 5. Record for deduplication
	if p.dedup != nil {
		p.dedup.Record(input)
	}

	return event
}

func (p *pipeline) buildEvent(input *Input) *Event {
	event := &Event{
		Type:      input.IssueType,
		Name:      input.Pod.Name,
		Namespace: input.Pod.Namespace,
		Reason:    input.Reason,
		Message:   input.Message,
		Logs:      input.Logs,
		Labels:    input.Pod.Labels,
	}

	if input.Container != nil {
		event.Container = input.Container.Name
	}

	if input.Owner != nil {
		event.Name = input.Owner.Name
	}

	return event
}

func (p *pipeline) buildNodeEvent(input *Input) *Event {
	return &Event{
		Type:    "node",
		Name:    input.Node.Name,
		Reason:  input.Reason,
		Message: input.Message,
		Labels:  input.Node.Labels,
	}
}

// Ensure pipeline implements Pipeline interface
var _ Pipeline = (*pipeline)(nil)
