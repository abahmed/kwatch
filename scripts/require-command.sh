#!/bin/sh

# Shared prerequisite checks for repository validation scripts.
require_command() {
	command_name=$1
	if ! command -v "$command_name" >/dev/null 2>&1; then
		printf '%s\n' \
			"$command_name is required; install ripgrep before running this check" \
			>&2
		return 127
	fi
}
