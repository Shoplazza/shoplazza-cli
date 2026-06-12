#!/bin/bash
# Shoplazza CLI installer — detects OS/arch, downloads the latest release binary,
# and installs it to /usr/local/bin (or a custom directory via INSTALL_DIR).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Shoplazza/shoplazza-cli/main/install.sh | bash
#
# Environment overrides:
#   INSTALL_DIR  — installation directory (default: /usr/local/bin)
#   VERSION      — specific version to install, e.g. "2.0.0" (default: latest)

set -euo pipefail

REPO="Shoplazza/shoplazza-cli"
BINARY="shoplazza"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
tmp_dir=""

# ── Detect OS ─────────────────────────────────────────────────────────────────

detect_os() {
  local os
  os="$(uname -s)"
  case "$os" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux" ;;
    *)
      echo "Error: unsupported OS: $os" >&2
      echo "Please download manually: https://github.com/$REPO/releases" >&2
      exit 1
      ;;
  esac
}

# ── Detect architecture ──────────────────────────────────────────────────────

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64)          echo "amd64" ;;
    amd64)           echo "amd64" ;;
    arm64|aarch64)   echo "arm64" ;;
    *)
      echo "Error: unsupported architecture: $arch" >&2
      echo "Please download manually: https://github.com/$REPO/releases" >&2
      exit 1
      ;;
  esac
}

# ── Fetch latest version ─────────────────────────────────────────────────────

fetch_latest_version() {
  local url="https://api.github.com/repos/$REPO/releases/latest"
  local tag

  if command -v curl >/dev/null 2>&1; then
    tag="$(curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"v\([^"]*\)".*/\1/')"
  elif command -v wget >/dev/null 2>&1; then
    tag="$(wget -qO- "$url" | grep '"tag_name"' | head -1 | sed 's/.*"v\([^"]*\)".*/\1/')"
  else
    echo "Error: curl or wget is required" >&2
    exit 1
  fi

  if [ -z "$tag" ]; then
    echo "Error: failed to fetch latest version from GitHub" >&2
    exit 1
  fi

  echo "$tag"
}

# ── Download ──────────────────────────────────────────────────────────────────

download() {
  local url="$1"
  local dest="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
  fi
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  local os arch version archive_name download_url

  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${VERSION:-$(fetch_latest_version)}"

  archive_name="shoplazza-cli-${version}-${os}-${arch}.tar.gz"
  download_url="https://github.com/$REPO/releases/download/v${version}/${archive_name}"

  echo "Installing shoplazza-cli v${version} (${os}/${arch})..."

  # Download to temp directory
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  echo "Downloading ${download_url}..."
  download "$download_url" "$tmp_dir/$archive_name"

  # Extract
  tar -xzf "$tmp_dir/$archive_name" -C "$tmp_dir"

  # Find the binary (may be at top level or inside a subdirectory)
  local bin_path="$tmp_dir/$BINARY"
  if [ ! -f "$bin_path" ]; then
    bin_path="$(find "$tmp_dir" -name "$BINARY" -type f | head -1)"
  fi

  if [ -z "$bin_path" ] || [ ! -f "$bin_path" ]; then
    echo "Error: binary '$BINARY' not found in archive" >&2
    exit 1
  fi

  chmod +x "$bin_path"

  # Install
  if [ -w "$INSTALL_DIR" ]; then
    install -m755 "$bin_path" "$INSTALL_DIR/$BINARY"
  else
    echo "Need sudo to install to $INSTALL_DIR"
    sudo install -m755 "$bin_path" "$INSTALL_DIR/$BINARY"
  fi

  echo ""
  echo "Installed: $INSTALL_DIR/$BINARY"
  "$INSTALL_DIR/$BINARY" --version 2>/dev/null || true
  echo ""
  echo "Run 'shoplazza auth login --store-domain <your-store>' to get started."
}

main
