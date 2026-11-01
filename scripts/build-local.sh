#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Local SDK checkout lives in tmp/ (gitignored). Override with CPA_SRC if needed.
CPA_SRC="${CPA_SRC:-tmp/CLIProxyAPI-src}"

if [[ ! -d "$CPA_SRC" ]]; then
  mkdir -p "$(dirname "$CPA_SRC")"
  git clone --depth 1 https://github.com/router-for-me/CLIProxyAPI "$CPA_SRC"
fi

if ! grep -q 'replace github.com/router-for-me/CLIProxyAPI/v7' go.mod; then
  printf '\nreplace github.com/router-for-me/CLIProxyAPI/v7 => %s\n' "$CPA_SRC" >> go.mod
fi

go test ./internal/stickybind/ -count=1
mkdir -p bin
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -o "bin/cpa-resin-sticky-bind.so" .
echo "built bin/cpa-resin-sticky-bind.so"
