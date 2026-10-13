// Package selfcheck measures, after the fact, whether the cause kwatch
// named was the thing that changed when an incident ended. When an
// incident resolves it asks one question: did recovery follow a change to
// (or a recovery of) the cause entity, did something else change, or did
// nothing change? Counts are kept per propagation rule for the last seven
// days, a few bytes each, and shown on /status.
//
// The counts are display only. Nothing in root-cause scoring, tiers or
// messages reads them, so a rule that looks weak here is never made
// weaker, and one that looks strong is never trusted more.
package selfcheck
