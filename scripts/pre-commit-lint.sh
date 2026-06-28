#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/dev-env.sh
source "$(dirname "$0")/dev-env.sh"
cd "${ROOT}"
exec golangci-lint run --fix --timeout=5m
