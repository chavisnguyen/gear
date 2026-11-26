#!/usr/bin/env bash
set -euo pipefail

REPO="chavisnguyen/gear"
INSTALL_DIR="${GEAR_INSTALL_DIR:-$HOME/.gear/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TAG="${1:-}"
if [ -z "$TAG" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep -Eo '"tag_name":[[:space:]]*"[^"]+"' | head -1 | cut -d'"' -f4)"
fi

ARCHIVE="gear-${OS}-${ARCH}.tar.gz"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "https://github.com/$REPO/releases/download/$TAG/$ARCHIVE" -o "$TMP/$ARCHIVE"
curl -fsSL "https://github.com/$REPO/releases/download/$TAG/checksums.txt" -o "$TMP/checksums.txt"

EXPECTED="$(grep " $ARCHIVE\$" "$TMP/checksums.txt" | awk '{print $1}')"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')"
else
  ACTUAL="$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')"
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum mismatch for $ARCHIVE (expected $EXPECTED, got $ACTUAL)" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
install -m 0755 "$TMP/gear" "$INSTALL_DIR/gear"
echo "✅ Installed gear ($TAG) to $INSTALL_DIR/gear"
