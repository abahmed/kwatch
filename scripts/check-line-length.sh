#!/bin/sh

# Check newly added lines and complete untracked files. Existing legacy lines
# are reported when their file is intentionally refactored, not hidden by a
# repository-wide formatter pass.
set -eu

status=0

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

for file in $(git ls-files --others --exclude-standard -- "*.go"); do
	check_file "$file"
done

git diff --unified=0 -- '*.go' | awk '
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
