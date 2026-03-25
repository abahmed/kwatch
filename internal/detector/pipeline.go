package detector

import (
	"time"

	"github.com/sirupsen/logrus"
)

// Heuristic represents a heuristic rule
type Heuristic struct {
	Name  string
	Check func(input *Input) HeuristicResult
}

// HeuristicResult represents the result of heuristic evaluation
type HeuristicResult struct {
	Status   string // "ALERT", "WAIT", "SKIP"
	Reason   string
	WaitTime int
}

// PipelineConfig holds pipeline configuration
type PipelineConfig struct {
	Volume          Volume
	Client          interface{}
	Config          interface{}
	DedupWindow     time.Duration
	AggregateWindow time.Duration
	Heuristics      []Heuristic
}

// pipeline implements the detection pipeline
type pipeline struct {
	config     *PipelineConfig
	predicates []Predicate
	detectors  []Detector
	handlers   []Enricher
	dedup      Deduplication
	aggregator Aggregator
	heuristics []Heuristic
}

// NewPipeline creates a new pipeline
func NewPipeline(config *PipelineConfig) *pipeline {
	p := &pipeline{
		config:     config,
		predicates: []Predicate{},
		detectors:  []Detector{},
		handlers:   []Enricher{},
		heuristics: []Heuristic{},
	}
	if config.Heuristics != nil {
		p.heuristics = config.Heuristics
	}
	return p
}

// AddPredicate adds a predicate to the pipeline
func (p *pipeline) AddPredicate(predicate Predicate) {
	p.predicates = append(p.predicates, predicate)
}

// AddDetector adds a detector to the pipeline
func (p *pipeline) AddDetector(detector Detector) {
	p.detectors = append(p.detectors, detector)
}

// AddHandler adds an enricher to the pipeline
func (p *pipeline) AddHandler(enricher Enricher) {
	p.handlers = append(p.handlers, enricher)
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

	// 2.5. Evaluate heuristics
	if len(p.heuristics) > 0 {
		result := p.evaluateHeuristics(input)
		switch result.Status {
		case "SKIP":
			logrus.Debugf("Pod skipped by heuristics: %s", result.Reason)
			return nil
		case "WAIT":
			logrus.Debugf("Pod waiting by heuristics: %s", result.Reason)
			return nil
		case "ALERT":
			logrus.Debugf("Pod alert confirmed by heuristics: %s", result.Reason)
		}
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

func (p *pipeline) evaluateHeuristics(input *Input) HeuristicResult {
	for _, h := range p.heuristics {
		result := h.Check(input)
		if result.Status != "SKIP" {
			return result
		}
	}
	return HeuristicResult{Status: "ALERT", Reason: "default"}
}

// Ensure pipeline implements Pipeline interface
var _ Pipeline = (*pipeline)(nil)

// DefaultHeuristics returns the default set of heuristic rules
func DefaultHeuristics() []Heuristic {
	return []Heuristic{
		{
			Name: "GracefulExit",
			Check: func(i *Input) HeuristicResult {
				if i.ExitCode == 0 || i.ExitCode == 143 {
					return HeuristicResult{Status: "SKIP", Reason: "graceful exit"}
				}
				return HeuristicResult{Status: "SKIP", Reason: "not graceful"}
			},
		},
		{
			Name: "ClearFailure",
			Check: func(i *Input) HeuristicResult {
				if i.RestartCount >= 3 {
					return HeuristicResult{Status: "ALERT", Reason: "3+ restarts"}
				}
				return HeuristicResult{Status: "SKIP", Reason: "low restart count"}
			},
		},
	}
}
