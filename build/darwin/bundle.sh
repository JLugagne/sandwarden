#!/bin/sh
# Wraps a built sandwarden binary into sandwarden.app so macOS gives it a bundle
# identifier, which native notifications require. Usage:
#   build/darwin/bundle.sh <binary> <version> <output-dir>
set -e

BINARY="$1"
VERSION="$2"
OUT="$3"
HERE="$(cd "$(dirname "$0")" && pwd)"

case "$VERSION" in
  v[0-9]*) PLIST_VERSION="${VERSION#v}" ;;
  *) PLIST_VERSION="0.0.0" ;;
esac

APP="$OUT/sandwarden.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BINARY" "$APP/Contents/MacOS/sandwarden"
chmod 0755 "$APP/Contents/MacOS/sandwarden"
sed "s/__VERSION__/$PLIST_VERSION/g" "$HERE/Info.plist" > "$APP/Contents/Info.plist"
codesign --force --deep --sign - "$APP"
echo "$APP"
