#!/usr/bin/env sh
# Installer for the dare CLI.
# Usage: curl -fsSL https://dare.dev/install.sh | sh
#
# Downloads the correct prebuilt binary for the user's OS/arch from
# GitHub Releases and puts it on PATH. No API key, no config required.

set -eu

REPO="baalebos-cloud/dare"
BIN_NAME="dare"
INSTALL_DIR="${DARE_INSTALL_DIR:-$HOME/.local/bin}"

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) echo "unsupported"; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "unsupported"; exit 1 ;;
  esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
EXT="tar.gz"
[ "$OS" = "windows" ] && EXT="zip"

LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"

if [ -z "$LATEST_TAG" ]; then
  echo "Could not determine latest release. Check https://github.com/${REPO}/releases" >&2
  exit 1
fi

ASSET="${BIN_NAME}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${ASSET}"

echo "Installing ${BIN_NAME} ${LATEST_TAG} (${OS}/${ARCH})..."

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$URL" -o "$TMP_DIR/$ASSET"

mkdir -p "$INSTALL_DIR"

if [ "$EXT" = "tar.gz" ]; then
  tar -xzf "$TMP_DIR/$ASSET" -C "$TMP_DIR"
else
  unzip -q "$TMP_DIR/$ASSET" -d "$TMP_DIR"
fi

mv "$TMP_DIR/${BIN_NAME}" "$INSTALL_DIR/${BIN_NAME}"
chmod +x "$INSTALL_DIR/${BIN_NAME}"

echo "Installed to ${INSTALL_DIR}/${BIN_NAME}"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
    echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
    ;;
esac

echo ""
echo "Run 'dare --help' to get started."
echo "By default it looks for a local Ollama server, then falls back to"
echo "a keyless gateway. No API key needed. Use --offline to disable the gateway."
