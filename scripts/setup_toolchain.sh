#!/bin/bash
set -e

# Resolve repo root
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# OS and Arch checks
OS="$(uname -s)"
ARCH="$(uname -m)"

if [ "$OS" != "Darwin" ] || [ "$ARCH" != "arm64" ]; then
  echo "Error: This local toolchain script is specifically for macOS ARM64."
  echo "Please install Node.js manually for your platform."
  exit 1
fi

# Setup the .tools directory
mkdir -p "$REPO_ROOT/.tools"
cd "$REPO_ROOT/.tools"

# Download and extract Node.js for macOS ARM64
echo "Downloading Node.js v20.15.0 for macOS ARM64..."
curl -sS -O https://nodejs.org/dist/v20.15.0/node-v20.15.0-darwin-arm64.tar.gz

echo "Extracting Node.js..."
tar -xzf node-v20.15.0-darwin-arm64.tar.gz
rm node-v20.15.0-darwin-arm64.tar.gz

# Rename folder to just 'node'
rm -rf node
mv node-v20.15.0-darwin-arm64 node

echo ""
echo "✅ Node.js successfully installed in $REPO_ROOT/.tools/node"
echo "To use it, run the following command in your terminal:"
echo ""
echo "export PATH=\"$REPO_ROOT/.tools/node/bin:\$PATH\""
echo ""
