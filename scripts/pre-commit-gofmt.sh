#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/dev-env.sh
source "$(dirname "$0")/dev-env.sh"
exec gofmt -s -w "$@"
