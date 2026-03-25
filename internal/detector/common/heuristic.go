package common

import (
	"github.com/abahmed/kwatch/internal/detector"
)

// HeuristicStatus represents the result of heuristic evaluation
type HeuristicStatus string

const (
	// HeuristicStatusAlert means issue is confirmed, send alert
	HeuristicStatusAlert HeuristicStatus = "ALERT"
	// HeuristicStatusWait means not enough data, wait for more
	HeuristicStatusWait HeuristicStatus = "WAIT"
	// HeuristicStatusSkip means normal/expected, don't alert
	HeuristicStatusSkip HeuristicStatus = "SKIP"
)

// Heuristic represents a single heuristic rule
type Heuristic struct {
	Name        string
	Description string
	Check       func(input *detector.Input) HeuristicResult
}

// HeuristicResult contains the result of heuristic evaluation
type HeuristicResult struct {
	Status     HeuristicStatus
	Reason     string
	WaitTime   int // seconds
	WaitEvents int
}

// DefaultHeuristics returns the default set of heuristic rules
func DefaultHeuristics() []Heuristic {
	return []Heuristic{
		{
			Name:        "ClearFailure",
			Description: "Clear failure with multiple restarts",
			Check: func(i *detector.Input) HeuristicResult {
				if IsClearFailure(i.Reason) && i.RestartCount >= 3 {
					return HeuristicResult{
						Status: HeuristicStatusAlert,
						Reason: "Clear failure with 3+ restarts",
					}
				}
				if IsClearFailure(i.Reason) {
					return HeuristicResult{
						Status:   HeuristicStatusWait,
						Reason:   "Clear failure but low restart count",
						WaitTime: 120,
					}
				}
				return HeuristicResult{Status: HeuristicStatusSkip}
			},
		},
		{
			Name:        "GracefulExit",
			Description: "Graceful termination",
			Check: func(i *detector.Input) HeuristicResult {
				if IsGracefulExit(i.ExitCode) {
					return HeuristicResult{
						Status: HeuristicStatusSkip,
						Reason: "Graceful termination",
					}
				}
				return HeuristicResult{Status: HeuristicStatusSkip}
			},
		},
		{
			Name:        "StartupDelay",
			Description: "Normal startup delay",
			Check: func(i *detector.Input) HeuristicResult {
				startupReasons := []string{
					"ContainerCreating",
					"PodInitializing",
					"BackOff",
				}
				for _, r := range startupReasons {
					if i.Reason == r {
						return HeuristicResult{
							Status:   HeuristicStatusWait,
							Reason:   "Normal startup",
							WaitTime: 300,
						}
					}
				}
				return HeuristicResult{Status: HeuristicStatusSkip}
			},
		},
		{
			Name:        "NoData",
			Description: "Not enough data to decide",
			Check: func(i *detector.Input) HeuristicResult {
				if i.Reason == "" && i.RestartCount == 0 {
					return HeuristicResult{
						Status:   HeuristicStatusWait,
						Reason:   "Need more data",
						WaitTime: 180,
					}
				}
				return HeuristicResult{Status: HeuristicStatusSkip}
			},
		},
	}
}

// EvaluateHeuristics runs all heuristics and returns the first non-skip result
func EvaluateHeuristics(input *detector.Input, heuristics []Heuristic) HeuristicResult {
	for _, h := range heuristics {
		result := h.Check(input)
		if result.Status != HeuristicStatusSkip {
			return result
		}
	}
	// Default: alert if no heuristic matched
	return HeuristicResult{
		Status: HeuristicStatusAlert,
		Reason: "Default alert",
	}
}
