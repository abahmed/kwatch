#!/bin/sh

set -eu

echo "Checking alert safety regressions..."
go test ./internal/message -run \
	'TestNotificationOmitsWeakCauseAndUnsafeGroupCommand|TestProviderRenderersShareSafeHumanSemantics|TestRedactEvidenceRemovesCredentialsAndPrivateAddresses'

echo "Checking Slack retry and conversation regressions..."
go test ./internal/alert/slack -run \
	'TestStructuredNotificationRetryDoesNotDuplicateRoot|TestSlackStructuredConversationStateRoundTrips'

echo "Checking persistence and lifecycle regressions..."
go test ./internal/persistence -run \
	'TestSaveChangeHistoryCompactsOneOversizedEntry|TestProductionRegressionFixtureCompactsOversizedChange|TestStartupAnnouncementClaimIsAtomic'
go test ./internal/incident -run \
	'TestMarkResolvedIdempotent|TestSmartGroupingBuffersSameReason|TestSharedMetricsFailureUsesGlobalGroupOnlyWithEvidence'

echo "Negative regression checks passed."
