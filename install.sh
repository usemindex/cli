#!/bin/sh
set -e

REPO="usemindex/cli"
BINARY="mindex"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

URL="https://github.com/${REPO}/releases/latest/download/${BINARY}_${OS}_${ARCH}.tar.gz"

echo "Installing ${BINARY}..."
curl -fsSL "$URL" | tar xz -C /tmp "${BINARY}"
sudo mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
echo "✓ ${BINARY} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Get started:"
echo "  mindex auth"
echo "  mindex context \"your question here\""
