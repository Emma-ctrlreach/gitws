#!/usr/bin/env zsh

set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH"
  exit 1
fi

gobin=$(go env GOBIN)
if [[ -z "$gobin" ]]; then
  gobin="$(go env GOPATH)/bin"
fi

target="$gobin/gitws"

if [[ -f "$target" || -L "$target" ]]; then
  rm -f "$target"
  echo "Removed: $target"
else
  echo "gitws not found at: $target"
fi

echo
echo "If gitws still resolves, check for another copy with:"
echo "  which -a gitws"
