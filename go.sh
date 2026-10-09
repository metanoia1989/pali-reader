#!/usr/bin/env bash
# Runs a Go command with the in-workspace module cache.
#   ./go.sh build ./...      (run from backend/)
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=goenv.sh
source "$here/goenv.sh"
exec go "$@"
