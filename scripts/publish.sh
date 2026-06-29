#!/usr/bin/env bash
# Bump version (optional), test, tag, push, GitHub release, go install smoke test.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
# shellcheck source=scripts/dev-env.sh
source "${ROOT}/scripts/dev-env.sh"

BUMP="${1:-}"
VERSION_FILE="cmd/scip-cli/main.go"
MODULE_INSTALL="github.com/flesler/scip-cli-go/v2/cmd/scip-cli"
SMOKE_ATTEMPTS="${SCIP_CLI_PUBLISH_SMOKE_ATTEMPTS:-12}"
SMOKE_SLEEP_SEC="${SCIP_CLI_PUBLISH_SMOKE_SLEEP_SEC:-15}"

usage() {
	cat <<'EOF'
Usage: scripts/publish.sh [patch|minor|major]

  patch|minor|major  Bump cmd/scip-cli/main.go version, commit, then publish
  (no argument)      Publish the current version without bumping

Runs scripts/test.sh, creates git tag vX.Y.Z, pushes, gh release, then:
  go install github.com/flesler/scip-cli-go/v2/cmd/scip-cli@vX.Y.Z
  scip-cli --version  (must report scip-cli-go X.Y.Z)
EOF
	exit 1
}

case "${BUMP}" in
"" | patch | minor | major) ;;
-h | --help) usage ;;
*)
	echo "Error: unknown bump level '${BUMP}' (expected patch, minor, or major)" >&2
	usage
	;;
esac

read_version() {
	grep -E '^const version = ' "${VERSION_FILE}" | sed -E 's/.*"([^"]+)".*/\1/'
}

bump_version() {
	local level="$1"
	local current major minor patch new
	current="$(read_version)"
	IFS=. read -r major minor patch <<<"${current}"
	case "${level}" in
	patch) patch=$((patch + 1)) ;;
	minor)
		minor=$((minor + 1))
		patch=0
		;;
	major)
		major=$((major + 1))
		minor=0
		patch=0
		;;
	*)
		echo "Error: invalid bump level: ${level}" >&2
		exit 1
		;;
	esac
	new="${major}.${minor}.${patch}"
	sed -i "s/^const version = \".*\"/const version = \"${new}\"/" "${VERSION_FILE}"
	printf '%s' "${new}"
}

"${ROOT}/scripts/test.sh"

if [[ -n "${BUMP}" ]]; then
	if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
		echo "Error: working tree has uncommitted changes; commit or stash before bumping" >&2
		git status --short
		exit 1
	fi

	echo "Bumping ${BUMP} version..."
	NEW_VERSION="$(bump_version "${BUMP}")"
	echo "New version: ${NEW_VERSION}"

	git add "${VERSION_FILE}"
	git commit -m "Release ${NEW_VERSION}."
fi

make build

VERSION="$(read_version)"
echo "Publishing version ${VERSION}..."

LOCAL_OUT="$(./bin/scip-cli-go --version)"
if [[ "${LOCAL_OUT}" != *"scip-cli-go ${VERSION}"* ]]; then
	echo "Error: local binary version mismatch: ${LOCAL_OUT}" >&2
	exit 1
fi

if git rev-parse "v${VERSION}" >/dev/null 2>&1; then
	echo "Error: tag v${VERSION} already exists" >&2
	exit 1
fi

echo "Creating git tag v${VERSION}..."
git tag -a "v${VERSION}" -m "Release ${VERSION}."
git push origin HEAD "v${VERSION}"

REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || true)"
if [[ -z "${REPO}" ]]; then
	REPO="flesler/scip-cli-go"
fi

if gh release view "v${VERSION}" -R "${REPO}" >/dev/null 2>&1; then
	echo "GitHub release v${VERSION} already exists"
else
	echo "Creating GitHub release v${VERSION}..."
	gh release create "v${VERSION}" -R "${REPO}" --title "v${VERSION}" --notes "$(cat <<EOF
Release ${VERSION}.

\`\`\`bash
go install ${MODULE_INSTALL}@v${VERSION}
\`\`\`
EOF
)"
fi

echo "Smoke testing go install @v${VERSION}..."
SMOKE_DIR="$(mktemp -d)"
cleanup() {
	rm -rf "${SMOKE_DIR}"
}
trap cleanup EXIT

installed=0
for ((i = 1; i <= SMOKE_ATTEMPTS; i++)); do
	if GOBIN="${SMOKE_DIR}" go install "${MODULE_INSTALL}@v${VERSION}"; then
		installed=1
		break
	fi
	if [[ "${i}" -eq "${SMOKE_ATTEMPTS}" ]]; then
		echo "Error: go install failed after ${SMOKE_ATTEMPTS} attempts (module proxy may be slow)" >&2
		exit 1
	fi
	echo "Waiting for module proxy (attempt ${i}/${SMOKE_ATTEMPTS})..."
	sleep "${SMOKE_SLEEP_SEC}"
done

if [[ "${installed}" -ne 1 ]]; then
	echo "Error: go install smoke test failed" >&2
	exit 1
fi

SMOKE_OUT="$("${SMOKE_DIR}/scip-cli" --version)"
if [[ "${SMOKE_OUT}" != *"scip-cli-go ${VERSION}"* ]]; then
	echo "Error: unexpected installed version: ${SMOKE_OUT}" >&2
	exit 1
fi

echo "Published v${VERSION} successfully!"
echo "Smoke test OK: ${SMOKE_OUT}"
echo "View at: https://github.com/${REPO}/releases/tag/v${VERSION}"
