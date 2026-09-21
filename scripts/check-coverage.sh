#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
	printf 'usage: %s coverage-profile\n' "$0" >&2
	exit 2
fi

profile=$1
if [ ! -s "$profile" ]; then
	printf 'coverage profile is missing or empty: %s\n' "$profile" >&2
	exit 2
fi

aggregate_min=75
core_min=70
core_packages='github.com/abahmed/kwatch/internal/app
github.com/abahmed/kwatch/internal/controller
github.com/abahmed/kwatch/internal/incident
github.com/abahmed/kwatch/internal/delivery
github.com/abahmed/kwatch/internal/delivery/transport
github.com/abahmed/kwatch/internal/persistence
github.com/abahmed/kwatch/internal/monitor/pod
github.com/abahmed/kwatch/internal/monitor/workload
github.com/abahmed/kwatch/internal/monitor/node
github.com/abahmed/kwatch/internal/monitor/network
github.com/abahmed/kwatch/internal/monitor/cluster
github.com/abahmed/kwatch/internal/monitor/security
github.com/abahmed/kwatch/internal/probe
github.com/abahmed/kwatch/internal/metricsapi
github.com/abahmed/kwatch/internal/kubeletmetrics
github.com/abahmed/kwatch/internal/statuswatch
github.com/abahmed/kwatch/internal/resource
github.com/abahmed/kwatch/internal/observe
github.com/abahmed/kwatch/internal/pvc
github.com/abahmed/kwatch/internal/crdwatch
github.com/abahmed/kwatch/internal/controlplane
github.com/abahmed/kwatch/internal/rbac'

summary=$(mktemp)
core_file=$(mktemp)
trap 'rm -f "$summary" "$core_file"' EXIT
printf '%s\n' "$core_packages" >"$core_file"

awk '
NR == 1 { next }
{
	key = $1
	file = key
	sub(/:[0-9][0-9]*\..*/, "", file)
	if (file ~ /\/zz_generated\.deepcopy\.go$/) {
		next
	}
	if (!(key in blockStatements)) {
		blockFile[key] = file
		blockStatements[key] = $2
	}
	if ($3 > 0) {
		blockCovered[key] = 1
	}
}
END {
	for (key in blockStatements) {
		file = blockFile[key]
		n = split(file, parts, "/")
		pkg = parts[1]
		for (i = 2; i < n; i++) {
			pkg = pkg "/" parts[i]
		}
		statements[pkg] += blockStatements[key]
		if (key in blockCovered) {
			covered[pkg] += blockStatements[key]
		}
	}
	for (pkg in statements) {
		printf "%s\t%d\t%d\n", pkg, statements[pkg], covered[pkg]
	}
}
' "$profile" | sort >"$summary"

read -r total covered <<EOF
$(awk -F '\t' '{total += $2; covered += $3} END {
	print total, covered
}' "$summary")
EOF

aggregate=$(awk -v covered="$covered" -v total="$total" \
	'BEGIN { if (total == 0) exit 1; printf "%.1f", covered * 100 / total }')
printf 'aggregate coverage: %s%% (minimum %s%%)\n' \
	"$aggregate" "$aggregate_min"

failed=0
if ! awk -v coverage="$aggregate" -v minimum="$aggregate_min" \
	'BEGIN { exit !(coverage + 0 >= minimum + 0) }'; then
	printf 'aggregate coverage is below the required minimum\n' >&2
	failed=1
fi

while IFS= read -r package; do
	[ -n "$package" ] || continue
	package_statements=$(awk -F '\t' -v package="$package" \
		'$1 == package {print $2}' "$summary")
	package_covered=$(awk -F '\t' -v package="$package" \
		'$1 == package {print $3}' "$summary")
	if [ -z "$package_statements" ]; then
		printf 'core package missing from coverage profile: %s\n' "$package" >&2
		exit 1
	fi
	package_coverage=$(awk \
		-v covered="$package_covered" -v total="$package_statements" \
		'BEGIN { printf "%.1f", covered * 100 / total }')
	printf 'core coverage: %s%% %s\n' "$package_coverage" "$package"
	if ! awk -v coverage="$package_coverage" -v minimum="$core_min" \
		'BEGIN { exit !(coverage + 0 >= minimum + 0) }'; then
		printf 'core package is below the required minimum: %s\n' \
			"$package" >&2
		failed=1
	fi
done <"$core_file"

exit "$failed"
