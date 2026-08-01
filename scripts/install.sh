#!/bin/sh
# PunchPage installer for macOS and Linux.
#   curl -fsSL https://punchpage.pages.dev/install.sh | sh
set -eu

REPO="thewh1teagle/punchpage"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin | linux) ;;
  *) echo "unsupported OS: $os (use the PowerShell installer on Windows)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

url="https://github.com/$REPO/releases/latest/download/punch_${os}_${arch}.tar.gz"
bin_dir="${PUNCHPAGE_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$bin_dir"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $url"
curl -fsSL "$url" | tar -xz -C "$tmp"
install -m 755 "$tmp/punch" "$bin_dir/punch"

echo "Installed punch to $bin_dir/punch"
"$bin_dir/punch" --help >/dev/null 2>&1 || true

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "NOTE: $bin_dir is not in your PATH. Add this to your shell profile:"
     echo "  export PATH=\"$bin_dir:\$PATH\"" ;;
esac
