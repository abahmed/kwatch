#!/bin/sh

# Check Go lines added since the merge base with BASE (default origin/main):
# committed, staged and unstaged changes, plus complete untracked files.
# Existing legacy lines are reported when their file is intentionally
# refactored, not hidden by a repository-wide formatter pass. When BASE does
# not resolve (a shallow clone or no remote), only uncommitted changes are
# checked.
set -eu
# pipefail is not POSIX; enable it where the shell supports it.
(set -o pipefail) 2>/dev/null && set -o pipefail

base_ref=${BASE:-origin/main}
status=0

if base=$(git merge-base "$base_ref" HEAD 2>/dev/null); then
	range=$base
else
	if [ "${CI:-}" = true ]; then
		echo "line-check: $base_ref not found; CI must not skip" >&2
		exit 1
	fi
	echo "line-check: $base_ref not found; checking uncommitted changes" >&2
	range=HEAD
fi

check_file() {
	file=$1
	awk -v file="$file" '
		length($0) > 80 {
			printf "%s:%d: line is %d columns (max 80)\n", file, FNR, length($0)
			error = 1
		}
		END { exit error }
	' "$file" || status=1
}

untracked=$(git ls-files --others --exclude-standard -- '*.go')
for file in $untracked; do
	check_file "$file"
done

git diff --unified=0 "$range" -- '*.go' | awk '
	/^\+\+\+ b\// {
		file = substr($0, 7)
		next
	}
	/^@@/ {
		match($0, /\+[0-9]+/)
		line = substr($0, RSTART + 1) + 0
		next
	}
	/^\+/ {
		value = substr($0, 2)
		if (length(value) > 80) {
			printf "%s:%d: line is %d columns (max 80)\n", file, line, length(value)
			error = 1
		}
		line++
	}
	END { exit error }
' || status=1

exit "$status"
