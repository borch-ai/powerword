#!/bin/bash
set -e

# Setup the .tools directory
mkdir -p .tools
cd .tools

# Download and extract Node.js for macOS ARM64
echo "Downloading Node.js v20.15.0 for macOS ARM64..."
curl -O https://nodejs.org/dist/v20.15.0/node-v20.15.0-darwin-arm64.tar.gz

echo "Extracting Node.js..."
tar -xzf node-v20.15.0-darwin-arm64.tar.gz
rm node-v20.15.0-darwin-arm64.tar.gz

# Rename folder to just 'node'
rm -rf node
mv node-v20.15.0-darwin-arm64 node

echo ""
echo "✅ Node.js successfully installed in $PWD/node"
echo "To use it, run the following command in your terminal:"
echo ""
echo "export PATH=\$PWD/node/bin:\$PATH"
echo ""
