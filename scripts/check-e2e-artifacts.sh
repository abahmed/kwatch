#!/bin/sh
# Scans E2E artifacts for secrets and canaries.
#
#   check-e2e-artifacts.sh PATH            exit 1 when unsafe text is found
#   check-e2e-artifacts.sh --redact PATH   replace each unsafe line in the
#                                          text files under PATH, then scan
#                                          again (exit 1 if anything is left)

set -eu

mode=check
if [ "${1:-}" = --redact ]; then
	mode=redact
	shift
fi
artifacts=${1:?artifact path is required}
if [ ! -e "$artifacts" ]; then
	echo "artifact path does not exist: $artifacts" >&2
	exit 2
fi

patterns='KWATCH_E2E_CANARY_|client-certificate-data:|client-key-data:|'
patterns="${patterns}Bearer [^[]|Authorization: Basic [^[]|e2e-token|"
patterns="${patterns}kind: Secret|diagnostics[_-]*token[=:][^[]|"
patterns="${patterns}password[=:][^[]|api[_-]*key[=:][^[]"

scan() {
	status=0
	grep -RIqiE --exclude='*.png' "$patterns" "$artifacts" || status=$?
	return "$status"
}

if [ "$mode" = redact ]; then
	# Only text files are rewritten (grep -I skips binary ones), one line
	# at a time, so the rest of the diagnostics stay readable.
	find "$artifacts" -type f ! -name '*.png' | while IFS= read -r file; do
		if grep -Iqi -E "$patterns" "$file" 2>/dev/null; then
			PATTERNS="$patterns" perl -pi -e '
				BEGIN { $re = qr/$ENV{PATTERNS}/i }
				$_ = "[redacted by the artifact safety check]\n" if /$re/;
			' "$file"
		fi
	done
fi

status=0
scan || status=$?
case "$status" in
0)
	echo "unsafe content found in E2E artifacts" >&2
	exit 1
	;;
1)
	;;
*)
	echo "scan error: grep exited with status $status" >&2
	exit 1
	;;
esac
