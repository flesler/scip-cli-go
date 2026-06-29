#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/dev-env.sh
source "$(dirname "$0")/dev-env.sh"
exec goimports -local github.com/flesler/scip-cli-go -w "$@"
