#!/usr/bin/env bash
# Publish gate: lint + unit tests (no e2e/cross — those need npx / Python scip-cli).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
# shellcheck source=scripts/dev-env.sh
source "${ROOT}/scripts/dev-env.sh"

make pre-commit test-unit
echo "All checks passed!"
