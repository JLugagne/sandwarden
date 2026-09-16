#!/bin/sh
# Install the rolling unstable pre-release built from the main branch.
set -e

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

curl -fsSL "https://raw.githubusercontent.com/JLugagne/sandwarden/main/install.sh" -o "$TMP"
sh "$TMP" unstable
