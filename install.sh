#!/usr/bin/env sh
#
# diffsmith installer — downloads the latest GitHub Release, verifies
# its checksum, and installs to /usr/local/bin (or $HOME/.local/bin if
# /usr/local/bin is not writable without sudo).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/selyafi/diffsmith/main/install.sh | sh
#
# Environment overrides:
#   DIFFSMITH_VERSION   Specific tag (e.g. v0.1.0). Default: latest release.
#   INSTALL_DIR         Target directory for the binary. Default: auto.
#
# Re-run the script at any time to upgrade — it always resolves to the
# latest release unless DIFFSMITH_VERSION is set.

set -eu

REPO="selyafi/diffsmith"
VERSION="${DIFFSMITH_VERSION:-latest}"

# --- output helpers (ANSI only when stdout is a tty) ----------------------
if [ -t 1 ]; then
  bold=$(printf '\033[1m')
  reset=$(printf '\033[0m')
  red=$(printf '\033[31m')
  green=$(printf '\033[32m')
else
  bold='' ; reset='' ; red='' ; green=''
fi

info() { printf '%s==>%s %s\n' "$bold" "$reset" "$*" ; }
ok()   { printf '%s ✓%s %s\n' "$green" "$reset" "$*" ; }
fail() { printf '%s ✗%s %s\n' "$red" "$reset" "$*" >&2 ; exit 1 ; }

# --- 1. detect platform ---------------------------------------------------
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             fail "Unsupported architecture: $ARCH (diffsmith ships amd64 and arm64)" ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) fail "Unsupported OS: $OS (diffsmith ships darwin and linux)" ;;
esac
if [ "$OS" = "linux" ] && [ "$ARCH" = "arm64" ]; then
  fail "linux/arm64 is not built in the current release. Build from source: go install github.com/$REPO/cmd/diffsmith@latest"
fi

# --- 2. resolve version ---------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  info "Resolving latest release tag..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
            | grep '"tag_name"' | head -1 | cut -d '"' -f 4) \
    || fail "Could not fetch latest release. Set DIFFSMITH_VERSION manually if you know the tag."
fi
[ -n "$VERSION" ] || fail "Could not resolve release tag."
# goreleaser strips the leading 'v' in artifact filenames.
VERSION_NO_V="${VERSION#v}"

# --- 3. choose install directory ------------------------------------------
if [ -n "${INSTALL_DIR:-}" ]; then
  TARGET_DIR="$INSTALL_DIR"
elif [ -w /usr/local/bin ] 2>/dev/null; then
  TARGET_DIR="/usr/local/bin"
else
  TARGET_DIR="$HOME/.local/bin"
fi
mkdir -p "$TARGET_DIR" || fail "Cannot create $TARGET_DIR"

# --- 4. download tarball + checksums --------------------------------------
TARBALL="diffsmith_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading $TARBALL ($VERSION) ..."
curl -fsSL "$BASE_URL/$TARBALL"   -o "$TMP/$TARBALL"   || fail "Failed to download $TARBALL"
curl -fsSL "$BASE_URL/SHA256SUMS" -o "$TMP/SHA256SUMS" || fail "Failed to download SHA256SUMS"

# --- 5. verify SHA256 -----------------------------------------------------
info "Verifying checksum ..."
EXPECTED=$(grep " $TARBALL\$" "$TMP/SHA256SUMS" | cut -d ' ' -f 1)
[ -n "$EXPECTED" ] || fail "Checksum for $TARBALL not found in SHA256SUMS"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TMP/$TARBALL" | cut -d ' ' -f 1)
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$TMP/$TARBALL" | cut -d ' ' -f 1)
else
  fail "Neither sha256sum nor shasum found; cannot verify download."
fi
[ "$EXPECTED" = "$ACTUAL" ] || fail "Checksum mismatch! expected=$EXPECTED actual=$ACTUAL"

# --- 6. extract + install -------------------------------------------------
tar -xzf "$TMP/$TARBALL" -C "$TMP" || fail "Failed to extract tarball"
[ -f "$TMP/diffsmith" ] || fail "Tarball did not contain a diffsmith binary"

install -m 755 "$TMP/diffsmith" "$TARGET_DIR/diffsmith" \
  || fail "Failed to install to $TARGET_DIR (try sudo or set INSTALL_DIR)"

ok "Installed diffsmith $VERSION to $TARGET_DIR/diffsmith"

# --- 7. PATH advisory -----------------------------------------------------
case ":$PATH:" in
  *":$TARGET_DIR:"*) ;;
  *)
    printf '\n%sNote:%s %s is not on your PATH. Add this to your shell rc:\n  export PATH="%s:$PATH"\n' \
      "$bold" "$reset" "$TARGET_DIR" "$TARGET_DIR"
    ;;
esac

printf '\nVerify with: %sdiffsmith --version%s\n' "$bold" "$reset"
