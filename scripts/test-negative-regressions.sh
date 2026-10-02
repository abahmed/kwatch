#!/bin/sh

set -eu
# pipefail is not POSIX; enable it where the shell supports it.
(set -o pipefail) 2>/dev/null && set -o pipefail

# run_tests PACKAGE TEST...
#
# Runs exactly the named tests in PACKAGE. A renamed or deleted test must
# fail this script: "go test -run" with a pattern that matches nothing
# still passes ("no tests to run"), so every name is first looked up with
# "go test -list", and the run uses an anchored pattern.
run_tests() {
	pkg="$1"
	shift
	listed=$(go test -list '.*' "$pkg")
	pattern=""
	for name in "$@"; do
		if ! printf '%s\n' "$listed" | grep -Fxq "$name"; then
			echo "negative regressions: $pkg has no test $name" >&2
			exit 1
		fi
		pattern="${pattern:+$pattern|}$name"
	done
	go test -count=1 "$pkg" -run "^($pattern)\$"
}

echo "Checking alert safety regressions..."
run_tests ./internal/redact \
	TestRedactEvidenceRemovesCredentialsAndPrivateAddresses
run_tests ./internal/notification \
	TestNeutralizeMentionsBreaksBroadcasts
# A weak rule must not name a cause: the explanation stays below the
# confidence floor and reports "unknown".
run_tests ./internal/rootcause/explain \
	TestExplainReportsUnknownBelowFloor

echo "Checking Slack retry and thread regressions..."
run_tests ./internal/alert/slack \
	TestPostWithThreadFallbackRetriesToTopLevelOnStaleThread \
	TestSlackIncidentPostsShortRootThenNoteInThread \
	TestSlackRestoredThreadIsReused

echo "Checking incident lifecycle regressions..."
run_tests ./internal/incident \
	TestManagerRestoreDoesNotReannounce \
	TestManagerRecoveryBeforeAnnouncementStaysSilent \
	TestManagerForgetsResolvedIncidentsAfterRemember \
	TestManagerFlappingIncidentGoesQuietThenResolves
run_tests ./internal/pipeline \
	TestEngineFlappingWorkloadIsQuiet \
	TestEngineRecoveredIncidentResolvesAtHoldDeadline

echo "Checking state store and startup regressions..."
run_tests ./internal/app \
	TestThreadSaverRoundTripRestoresSameMap \
	TestDiskStateStartupAnnouncementClaimsOncePerKey
# A deposed leader's late writes, thread saves included, are rejected by
# the store's epoch fence.
run_tests ./internal/storage \
	TestStoreStaleWriterIsFencedAfterNewerClaim \
	TestStoreWriteBeforeClaimReturnsNotClaimed

echo "Negative regression checks passed."
