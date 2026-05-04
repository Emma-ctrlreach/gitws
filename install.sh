#!/usr/bin/env zsh

set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH"
  exit 1
fi

repo_dir=${0:A:h}
cd "$repo_dir"

echo "Installing gitws with go install..."
go install ./cmd/gitws

gobin=$(go env GOBIN)
if [[ -z "$gobin" ]]; then
  gobin="$(go env GOPATH)/bin"
fi

echo
echo "gitws installed in: $gobin"

if [[ ":$PATH:" != *":$gobin:"* ]]; then
  echo
  echo "Add this to ~/.zshrc if needed:"
  echo "export PATH=\"\$PATH:$gobin\""
  echo
  echo "Then reload your shell:"
  echo "source ~/.zshrc"
fi

echo
echo "Usage:"
echo "  gitws"
echo "  gitws --root /path/to/code"
echo "  gitws /path/to/code"
echo "  gitws --help"
echo
echo "Runtime deps expected in PATH: lazygit, opencode"
