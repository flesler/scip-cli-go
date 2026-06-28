#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/dev-env.sh
source "$(dirname "$0")/dev-env.sh"
cd "${ROOT}"
go mod tidy
if ! git diff --quiet go.mod go.sum; then
  echo "go.mod/go.sum not tidy — run: go mod tidy" >&2
  exit 1
fi
