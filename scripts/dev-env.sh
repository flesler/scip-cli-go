#!/usr/bin/env bash
# Source or eval from other scripts: sets repo-local PATH and Go toolchain.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export PATH="${ROOT}/bin:${PATH}"

if [[ ! -x "${ROOT}/bin/golangci-lint" || ! -x "${ROOT}/bin/goimports" ]]; then
  make -C "${ROOT}" tools
fi
