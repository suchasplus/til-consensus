#!/usr/bin/env bash

set -euo pipefail

mode="${1:-check}"
case "$mode" in
check | list | write) ;;
*)
	echo "usage: $0 [check|list|write]" >&2
	exit 2
	;;
esac

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

files=()
while IFS= read -r -d '' file; do
	if [[ -f "$file" ]]; then
		files+=("$file")
	fi
done < <(git ls-files --cached --others --exclude-standard -z -- '*.go')

if [[ "$mode" == "list" ]]; then
	if ((${#files[@]} > 0)); then
		printf '%s\0' "${files[@]}"
	fi
	exit 0
fi

if ((${#files[@]} == 0)); then
	exit 0
fi

if [[ "$mode" == "write" ]]; then
	gofmt -w "${files[@]}"
else
	gofmt -l "${files[@]}"
fi
