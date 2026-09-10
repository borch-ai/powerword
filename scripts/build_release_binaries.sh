#!/usr/bin/env bash
set -euo pipefail

# build_release_binaries.sh compiles powerword and all pure-Go MCP plugins
# across all supported release architectures.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

VERSION="${1:-${VERSION:-dev}}"
if [ "${VERSION}" = "dev" ]; then
  VERSION_TAG="dev"
else
  VERSION_NUM="${VERSION#v}"
  VERSION_TAG="v${VERSION_NUM}"
fi

echo "Building release binaries for version ${VERSION_TAG}..."

rm -rf dist
mkdir -p dist

TARGETS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

PKGS=()
for dir in cmd/powerword cmd/pw-mcp-*; do
  if [ ! -d "$dir" ]; then
    continue
  fi
  if out=$(CGO_ENABLED=0 go list "./$dir" 2>&1); then
    PKGS+=("./$dir")
  else
    # Distinguish expected build-constraint exclusions (e.g., CGO-dependent packages) from unexpected errors.
    if echo "$out" | grep -q "build constraints exclude all Go files"; then
      echo "Skipping ./${dir} (build constraints exclude pure-Go/CGO_ENABLED=0 compilation)."
    else
      echo "Error inspecting package ./${dir}: ${out}" >&2
      exit 1
    fi
  fi
done

if [ "${#PKGS[@]}" -eq 0 ]; then
  echo "Error: No buildable packages found under cmd/." >&2
  exit 1
fi

echo "Discovered ${#PKGS[@]} buildable packages."

for pkg in "${PKGS[@]}"; do
  bin_name=$(basename "$pkg")
  echo "Compiling ${bin_name}..."
  for target in "${TARGETS[@]}"; do
    os="${target%%/*}"
    arch="${target##*/}"
    ext=""
    if [ "$os" = "windows" ]; then
      ext=".exe"
    fi
    out="dist/${bin_name}-${os}-${arch}${ext}"
    # Injects version metadata for powerword (pkg/config.Version); plugins maintain their own protocol Implementation.Version.
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
      -ldflags "-X github.com/borch-ai/powerword/pkg/config.Version=${VERSION_TAG}" \
      -o "$out" "$pkg"
  done
done

echo "Release build completed successfully. Binaries generated in dist/."
