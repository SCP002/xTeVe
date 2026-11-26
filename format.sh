#!/usr/bin/env bash

set -uo pipefail

cd "$(dirname "$0")"

for tool in goimports golines golangci-lint; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'Missing %s. Rebuild the devcontainer to install it.\n' "$tool" >&2
		exit 1
	fi
done

# src/webUI.go is generated from html/ and holds the web UI as long base64 strings.
mapfile -t go_files < <(find . -name '*.go' -not -path './.git/*' -not -path './src/webUI.go')

printf 'Sorting imports\n'
goimports -w "${go_files[@]}" || exit $?

printf '\nFormatting\n'
golines --base-formatter gofmt -m 120 -w "${go_files[@]}" || exit $?

printf '\nApplying auto-fixes and reporting remaining issues\n'
golangci-lint run --fix --issues-exit-code=0 ./...
