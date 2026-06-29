#!/usr/bin/env bash
# Source or eval from other scripts: sets repo-local PATH and Go toolchain.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export PATH="${ROOT}/bin:${PATH}"

# Cursor agent shells use an isolated $HOME without ~/.gitconfig; use the real one.
if [[ ! -f "${HOME}/.gitconfig" ]]; then
	for candidate in "/home/${USER}/.gitconfig" "/home/flesler/.gitconfig"; do
		if [[ -f "${candidate}" ]]; then
			export GIT_CONFIG_GLOBAL="${candidate}"
			break
		fi
	done
fi

if [[ ! -x "${ROOT}/bin/golangci-lint" || ! -x "${ROOT}/bin/goimports" ]]; then
  make -C "${ROOT}" tools
fi
