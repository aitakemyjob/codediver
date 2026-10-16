#!/usr/bin/env sh
# CodeDiver installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/aitakemyjob/codediver/main/scripts/install.sh | sh
#
# Environment:
#   CODEDIVER_VERSION  release tag to install (default: latest)
#   CODEDIVER_REPO     GitHub repo (default: aitakemyjob/codediver)
#   PREFIX            where the `codediver` command goes (default: ~/.local)
set -eu

REPO="${CODEDIVER_REPO:-aitakemyjob/codediver}"
VERSION="${CODEDIVER_VERSION:-latest}"
PREFIX="${PREFIX:-$HOME/.local}"

say() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

need curl
need uname

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

if [ "$VERSION" = "latest" ]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fetch() {
  say "downloading $1"
  curl -fL --progress-bar "$base/$1" -o "$tmp/$1" || die "download failed: $base/$1"
  if curl -fsL "$base/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
    expected=$(grep " $1\$" "$tmp/checksums.txt" | awk '{print $1}')
    if [ -n "$expected" ]; then
      if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$1" | awk '{print $1}');
      else actual=$(shasum -a 256 "$tmp/$1" | awk '{print $1}'); fi
      [ "$expected" = "$actual" ] || die "checksum mismatch for $1"
      say "checksum ok"
    fi
  fi
}

case "$os" in
  linux)
    asset="codediver_linux_${arch}.tar.gz"
    fetch "$asset"
    tar -xzf "$tmp/$asset" -C "$tmp"
    mkdir -p "$PREFIX/bin" "$HOME/.local/share/applications" "$HOME/.local/share/icons/hicolor/512x512/apps"
    install -m 0755 "$tmp/codediver" "$PREFIX/bin/codediver"
    install -m 0644 "$tmp/codediver.png" "$HOME/.local/share/icons/hicolor/512x512/apps/codediver.png"
    sed "s|^Exec=.*|Exec=$PREFIX/bin/codediver|" "$tmp/codediver.desktop" > "$HOME/.local/share/applications/codediver.desktop"
    command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$HOME/.local/share/applications" || true
    if ! ldconfig -p 2>/dev/null | grep -q libwebkit2gtk-4.1; then
      say "note: CodeDiver needs WebKitGTK. On Debian/Ubuntu: sudo apt install libwebkit2gtk-4.1-0 libgtk-3-0"
    fi
    ;;
  darwin)
    asset="codediver_darwin_universal.zip"
    fetch "$asset"
    need unzip
    unzip -q "$tmp/$asset" -d "$tmp"
    dest="/Applications"
    [ -w "$dest" ] || dest="$HOME/Applications"
    mkdir -p "$dest"
    rm -rf "$dest/CodeDiver.app"
    mv "$tmp/CodeDiver.app" "$dest/"
    # Release builds are not notarized yet; clear the quarantine flag so
    # Gatekeeper lets the app open.
    xattr -dr com.apple.quarantine "$dest/CodeDiver.app" 2>/dev/null || true
    mkdir -p "$PREFIX/bin"
    ln -sf "$dest/CodeDiver.app/Contents/MacOS/codediver" "$PREFIX/bin/codediver"
    say "installed $dest/CodeDiver.app"
    ;;
  *)
    die "unsupported OS: $os (on Windows use scripts/install.ps1 or the installer .exe)"
    ;;
esac

say "installed $PREFIX/bin/codediver"
case ":$PATH:" in
  *":$PREFIX/bin:"*) ;;
  *) say "add $PREFIX/bin to your PATH to use the \`codediver\` command" ;;
esac
say "connect Claude Code:  claude mcp add --transport http codediver http://127.0.0.1:47474/mcp"
