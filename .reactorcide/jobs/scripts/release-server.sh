#!/usr/bin/env bash
#
# Server release. semver-tags analyzes commits under v1/ (scoped with
# --directories) and, on a real bump, creates + pushes a prefixed tag
# `v1/vX.Y.Z`. We then build + push the image and bump the chart appVersion.
# Independent of release-helm (separate prefixed tag sequence).
#
# The runnerbase image ships only curl/git/bash, so semver-tags, the docker CLI,
# crane, and gh are all curl-installed (the corndogs release-server pattern).
# Runs in the dir the job command cloned (an authed full clone of main).
set -euo pipefail

SEMVER_TAGS_VERSION="${SEMVER_TAGS_VERSION:-v0.4.0}"
GHCLI_VERSION="${GHCLI_VERSION:-2.63.2}"
CRANE_VERSION="${CRANE_VERSION:-0.20.3}"
DOCKER_VERSION="${DOCKER_VERSION:-27.5.1}"

export HOME="${HOME:-/root}"
LOCAL_BIN="${HOME}/.local/bin"
mkdir -p "${LOCAL_BIN}" "${HOME}/.docker"
export PATH="${LOCAL_BIN}:${PATH}"

git config user.name "catalystcommunityci"
git config user.email "ci@catalystcommunity.org"
git fetch --tags --force origin

echo "=== install semver-tags ${SEMVER_TAGS_VERSION} ==="
curl -fsSL "https://github.com/catalystcommunity/semver-tags/releases/download/${SEMVER_TAGS_VERSION}/semver-tags.tar.gz" -o /tmp/semver-tags.tar.gz
tar -xzf /tmp/semver-tags.tar.gz -C "${LOCAL_BIN}"
chmod +x "${LOCAL_BIN}/semver-tags"

echo "=== compute version bump for v1/ ==="
semver-tags run --output_json --directories v1 > /tmp/semver.txt 2>&1
OUTPUT=$(tail -1 /tmp/semver.txt)
NEW_TAG=$(echo "${OUTPUT}"   | grep -o '"New_release_git_tag":"[^"]*"'  | cut -d'"' -f4)
PUBLISHED=$(echo "${OUTPUT}" | grep -o '"New_release_published":"[^"]*"' | cut -d'"' -f4)

if [ "${PUBLISHED}" != "true" ]; then
  echo "No new server release needed."
  exit 0
fi
# tag is "v1/vX.Y.Z" -> VERSION "X.Y.Z"
VERSION="${NEW_TAG##*/}"
VERSION="${VERSION#v}"
echo "=== releasing ${NEW_TAG} (version ${VERSION}) ==="

IMAGE="${REGISTRY}/${IMAGE_PATH}"

echo "=== install crane ==="
if ! command -v crane >/dev/null 2>&1; then
  curl -fsSL "https://github.com/google/go-containerregistry/releases/download/v${CRANE_VERSION}/go-containerregistry_Linux_x86_64.tar.gz" -o /tmp/crane.tar.gz
  tar -xzf /tmp/crane.tar.gz -C "${LOCAL_BIN}" crane
  rm /tmp/crane.tar.gz
fi

# Registry auth for crane (docker config.json).
AUTH=$(printf "%s:%s" "${REGISTRY_USER}" "${REGISTRY_PASSWORD}" | base64 -w 0)
cat > "${HOME}/.docker/config.json" <<EOF
{ "auths": { "${REGISTRY}": {"auth": "${AUTH}"} } }
EOF

echo "=== build + push ${IMAGE}:${VERSION} ==="
if [ -z "${DOCKER_HOST:-}" ]; then
  echo "ERROR: DOCKER_HOST not set (this job needs the 'docker' capability)" >&2
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL "https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz" -o /tmp/docker.tgz
  tar -xzf /tmp/docker.tgz --strip-components=1 -C "${LOCAL_BIN}" docker/docker
  rm /tmp/docker.tgz
fi
for _ in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 1; done

# Build from the repo root (the Dockerfile copies v1/). Pass the version ldflags
# the same way ./tools build-static does.
docker build \
  --build-arg "VERSION=${VERSION}" \
  --build-arg "GIT_COMMIT=$(git rev-parse --short HEAD)" \
  --build-arg "BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t "${IMAGE}:${VERSION}" .
docker save "${IMAGE}:${VERSION}" -o /tmp/image.tar
crane push /tmp/image.tar "${IMAGE}:${VERSION}"
crane push /tmp/image.tar "${IMAGE}:latest"
rm /tmp/image.tar

echo "=== bump chart appVersion to ${VERSION} ==="
sed -i "s/^appVersion: .*/appVersion: \"${VERSION}\"/" v1/charts/foundry-gateway-controller/Chart.yaml
git add v1/charts/foundry-gateway-controller/Chart.yaml
if ! git diff --cached --quiet; then
  git commit -m "ci: bump foundry appVersion to ${VERSION}"
  # release-helm may also be committing Chart.yaml (a different line); rebase to
  # serialize cleanly before pushing.
  git pull --rebase origin main || true
  git push origin main
fi

echo "=== create GitHub release ${NEW_TAG} ==="
if ! command -v gh >/dev/null 2>&1; then
  curl -fsSL "https://github.com/cli/cli/releases/download/v${GHCLI_VERSION}/gh_${GHCLI_VERSION}_linux_amd64.tar.gz" -o /tmp/gh.tar.gz
  tar -xzf /tmp/gh.tar.gz -C /tmp
  cp "/tmp/gh_${GHCLI_VERSION}_linux_amd64/bin/gh" "${LOCAL_BIN}/gh"
fi
GH_TOKEN="${GITHUB_PAT}" gh release create "${NEW_TAG}" \
  --repo "${REACTORCIDE_REPO}" \
  --title "${NEW_TAG}" \
  --generate-notes

echo "=== released ${NEW_TAG} (image ${IMAGE}:${VERSION}) ==="
