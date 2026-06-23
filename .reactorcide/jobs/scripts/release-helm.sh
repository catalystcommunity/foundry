#!/usr/bin/env bash
#
# Helm chart release. semver-tags analyzes commits under the chart dir (scoped
# with --directories), creates + pushes a prefixed tag, then bumps Chart.yaml
# `version`, packages, and publishes a GitHub release with the .tgz. Independent
# of release-server (separate prefixed tag sequence).
#
# runnerbase ships only curl/git/bash, so semver-tags, helm, and gh are
# curl-installed (the corndogs release-helm pattern). Runs in the dir the job
# command cloned (an authed full clone of main).
set -euo pipefail

CHART_DIR="v1/charts/foundry-gateway-controller"
SEMVER_TAGS_VERSION="${SEMVER_TAGS_VERSION:-v0.4.0}"
GHCLI_VERSION="${GHCLI_VERSION:-2.63.2}"

export HOME="${HOME:-/root}"
LOCAL_BIN="${HOME}/.local/bin"
mkdir -p "${LOCAL_BIN}"
export PATH="${LOCAL_BIN}:${PATH}"

git config user.name "catalystcommunityci"
git config user.email "ci@catalystcommunity.org"
git fetch --tags --force origin

echo "=== install semver-tags ${SEMVER_TAGS_VERSION} ==="
curl -fsSL "https://github.com/catalystcommunity/semver-tags/releases/download/${SEMVER_TAGS_VERSION}/semver-tags.tar.gz" -o /tmp/semver-tags.tar.gz
tar -xzf /tmp/semver-tags.tar.gz -C "${LOCAL_BIN}"
chmod +x "${LOCAL_BIN}/semver-tags"

echo "=== compute version bump for ${CHART_DIR}/ ==="
semver-tags run --output_json --directories "${CHART_DIR}" > /tmp/semver.txt 2>&1
OUTPUT=$(tail -1 /tmp/semver.txt)
NEW_TAG=$(echo "${OUTPUT}"   | grep -o '"New_release_git_tag":"[^"]*"'  | cut -d'"' -f4)
PUBLISHED=$(echo "${OUTPUT}" | grep -o '"New_release_published":"[^"]*"' | cut -d'"' -f4)

if [ "${PUBLISHED}" != "true" ]; then
  echo "No new chart release needed."
  exit 0
fi
# tag is "<chart-dir>/vX.Y.Z" -> VERSION "X.Y.Z"
VERSION="${NEW_TAG##*/}"
VERSION="${VERSION#v}"
echo "=== releasing ${NEW_TAG} (version ${VERSION}) ==="

echo "=== bump chart version to ${VERSION} ==="
sed -i "s/^version: .*/version: \"${VERSION}\"/" "${CHART_DIR}/Chart.yaml"
git add "${CHART_DIR}/Chart.yaml"
if ! git diff --cached --quiet; then
  git commit -m "ci: bump chart version to ${VERSION}"
  # release-server may also be committing Chart.yaml (a different line); rebase to
  # serialize cleanly before pushing.
  git pull --rebase origin main || true
  git push origin main
fi

echo "=== install helm + package chart ==="
if ! command -v helm >/dev/null 2>&1; then
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \
    | USE_SUDO=false HELM_INSTALL_DIR="${LOCAL_BIN}" bash
fi
helm package "${CHART_DIR}"   # produces foundry-gateway-controller-${VERSION}.tgz in cwd

echo "=== publish GitHub release ${NEW_TAG} ==="
if ! command -v gh >/dev/null 2>&1; then
  curl -fsSL "https://github.com/cli/cli/releases/download/v${GHCLI_VERSION}/gh_${GHCLI_VERSION}_linux_amd64.tar.gz" -o /tmp/gh.tar.gz
  tar -xzf /tmp/gh.tar.gz -C /tmp
  cp "/tmp/gh_${GHCLI_VERSION}_linux_amd64/bin/gh" "${LOCAL_BIN}/gh"
fi
GH_TOKEN="${GITHUB_PAT}" gh release create "${NEW_TAG}" \
  --repo "${REACTORCIDE_REPO}" \
  --title "${NEW_TAG}" \
  --notes "Helm chart ${VERSION}" \
  ./foundry-gateway-controller-*.tgz

echo "=== released chart ${NEW_TAG} (version ${VERSION}) ==="
