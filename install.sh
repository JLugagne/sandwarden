#!/bin/sh
set -e

REPO="JLugagne/sandwarden"
BINARY="sandwarden"
INSTALL_DIR="${SANDWARDEN_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${SANDWARDEN_VERSION:-}"

# Resolve OS
OS="$(uname -s)"
case "$OS" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Resolve arch
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

if [ "$OS" = "linux" ] && [ "$ARCH" != "amd64" ]; then
  echo "Prebuilt Linux binaries are amd64 only; build from source with 'make build'." >&2
  exit 1
fi

# Resolve version: latest stable release unless pinned. SANDWARDEN_VERSION=unstable
# installs the rolling pre-release built from the main branch.
if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
fi

if [ -z "$VERSION" ]; then
  echo "Could not determine the latest version. Set SANDWARDEN_VERSION to override." >&2
  exit 1
fi

ARCHIVE="sandwarden_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Installing sandwarden ${VERSION} (${OS}/${ARCH})..."

curl -fsSL "$BASE/$ARCHIVE" -o "$TMP/$ARCHIVE"
curl -fsSL "$BASE/checksums.txt" -o "$TMP/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c - )
else
  ( cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | shasum -a 256 -c - )
fi

tar -xzf "$TMP/$ARCHIVE" -C "$TMP"

mkdir -p "$INSTALL_DIR"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
else
  mv "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
  chmod +x "$INSTALL_DIR/$BINARY"
fi

echo "sandwarden installed to $INSTALL_DIR/$BINARY"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo
    echo "$INSTALL_DIR is not in your PATH. Add it with:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

if [ "$OS" = "linux" ]; then
  echo
  echo "Linux runtime dependencies: libgtk-3-0 and libwebkit2gtk-4.1-0 (Debian/Ubuntu package names)."
fi
