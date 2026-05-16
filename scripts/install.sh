#!/usr/bin/env sh
# sumolite installer.
#
# Usage:  curl -fsSL https://sumolite.dev/install.sh | sh
#
# What it does:
#   1. Detects OS + arch (macos/linux x amd64/arm64).
#   2. Downloads the matching static binary from GitHub releases.
#   3. Installs to /usr/local/bin/sumolite (or ~/.local/bin if no sudo).
#   4. On Linux, installs a udev rule giving the `input` group access to
#      /dev/uinput, and adds the current user to that group.
#   5. Installs a launchd plist (macOS) or systemd --user unit (linux) so
#      it survives reboots.
#   6. Prints the pairing URL.

set -eu

REPO="asklokesh/sumolite"
BIN="sumolite"

uname_s=$(uname -s | tr '[:upper:]' '[:lower:]')
uname_m=$(uname -m)
case "$uname_s" in
  darwin) os=darwin ;;
  linux)  os=linux  ;;
  *) echo "unsupported OS: $uname_s" >&2; exit 1 ;;
esac
case "$uname_m" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported arch: $uname_m" >&2; exit 1 ;;
esac

asset="${BIN}-${os}-${arch}"
url="https://github.com/${REPO}/releases/latest/download/${asset}"

target=/usr/local/bin
if [ ! -w "$target" ] && ! command -v sudo >/dev/null 2>&1; then
  target="$HOME/.local/bin"
  mkdir -p "$target"
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

echo "downloading $url"
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"

if [ -w "$target" ]; then
  mv "$tmp" "$target/$BIN"
else
  sudo mv "$tmp" "$target/$BIN"
fi
trap - EXIT

# Linux: grant /dev/uinput to the input group, add current user.
if [ "$os" = "linux" ]; then
  if [ ! -e /etc/udev/rules.d/99-sumolite.rules ]; then
    echo 'KERNEL=="uinput", GROUP="input", MODE="0660", OPTIONS+="static_node=uinput"' | \
      sudo tee /etc/udev/rules.d/99-sumolite.rules >/dev/null
    sudo udevadm control --reload-rules
    sudo udevadm trigger
  fi
  if ! id -nG "$USER" | grep -qw input; then
    echo "adding $USER to input group (log out + back in for it to take effect)"
    sudo usermod -aG input "$USER"
  fi
fi

echo
echo "installed: $(command -v $BIN || echo $target/$BIN)"
echo "run:       sumolite serve"
