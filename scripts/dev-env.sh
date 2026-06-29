#!/usr/bin/env bash
# Source or eval from other scripts: sets repo-local PATH and Go toolchain.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export PATH="${ROOT}/bin:${PATH}"

# System /usr/bin/go is often 1.18; this project requires Go 1.25+ (see go.mod).
for gobin in "${GOSDK:-${HOME}/.local/go-sdk/go}/bin" "/home/flesler/.local/go-sdk/go/bin"; do
	if [[ -x "${gobin}/go" ]]; then
		export PATH="${gobin}:${PATH}"
		break
	fi
done

if ! go version 2>/dev/null | grep -qE 'go1\.(2[5-9]|[3-9][0-9])'; then
	echo "Error: Go 1.25+ required (found: $(go version 2>/dev/null || echo 'go not found'))" >&2
	echo "Install to ~/.local/go-sdk/go or put Go 1.25+ first on PATH." >&2
	exit 1
fi

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
