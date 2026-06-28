#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/dev-env.sh
source "$(dirname "$0")/dev-env.sh"
cd "${ROOT}"
go build -o /dev/null ./...
go vet ./...
