#!/bin/sh
set -e

REPO="JLugagne/sandwarden"
BINARY="sandwarden"
INSTALL_DIR="${SANDWARDEN_INSTALL_DIR:-$HOME/.local/bin}"

# Channel: latest (default), unstable, or a version tag such as v0.1.0.
CHANNEL="${1:-${SANDWARDEN_CHANNEL:-latest}}"
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

if [ "$OS" = "darwin" ] && [ "$ARCH" != "arm64" ]; then
  echo "Prebuilt macOS binaries are Apple silicon (arm64) only; build from source with 'make build'." >&2
  exit 1
fi

# Resolve the channel without the rate-limited API: the latest stable release
# page redirects to its tag. When no stable release exists yet (fresh repo) or
# the redirect cannot be read, fall back to the unstable pre-release.
resolve_latest() {
  target="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPO}/releases/latest" 2>/dev/null || true)"
  case "$target" in
    */releases/tag/*) printf '%s\n' "${target##*/tag/}" ;;
    *) return 1 ;;
  esac
}

if [ -z "$VERSION" ]; then
  case "$CHANNEL" in
    unstable)
      VERSION="unstable"
      ;;
    latest)
      if ! VERSION="$(resolve_latest)"; then
        echo "No stable release is published yet; installing the unstable pre-release."
        VERSION="unstable"
      fi
      ;;
    v*)
      VERSION="$CHANNEL"
      ;;
    *)
      echo "Unknown channel '$CHANNEL'. Use latest, unstable, or a version tag." >&2
      exit 1
      ;;
  esac
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
if [ "$OS" = "darwin" ]; then
  # The binary lives inside sandwarden.app so macOS gives it the bundle
  # identifier notifications need; the PATH entry only forwards to it.
  APP_DIR="${SANDWARDEN_APP_DIR:-$HOME/Applications}"
  mkdir -p "$APP_DIR"
  rm -rf "$APP_DIR/sandwarden.app"
  mv "$TMP/sandwarden.app" "$APP_DIR/sandwarden.app"
  rm -f "$INSTALL_DIR/$BINARY"
  printf '#!/bin/sh\nexec "%s/sandwarden.app/Contents/MacOS/sandwarden" "$@"\n' "$APP_DIR" > "$INSTALL_DIR/$BINARY"
  chmod 0755 "$INSTALL_DIR/$BINARY"
  echo "sandwarden.app installed to $APP_DIR"
elif command -v install >/dev/null 2>&1; then
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
