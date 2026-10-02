// Package scenarios is kwatch's labelled scenario library and scorecard
// gate. In: synthetic observation logs in testdata, each built from
// Kubernetes objects by a generator and paired with an expectation (the
// root that should be blamed, its tier, a message budget and what must
// never be blamed). Out: per-scenario verdicts and the alert-quality
// gates of docs/production-goals.md, measured by replaying every log, a
// 1,000-pod storm and a 12-hour synthetic staging day through a fresh
// engine. A separate held-out set (testdata/heldout) is scored on its
// own and never used to tune the engine; see heldout_test.go. It holds
// only tests; nothing imports it.
//
// Regenerate the committed logs after changing a scenario:
//
//	go test ./internal/scenarios -run TestScenarioFixtures -update
//
// Report the gates (never fails) or enforce them:
//
//	make alert-quality
//	make alert-quality-gate
package scenarios
