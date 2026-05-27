#!/usr/bin/env sh
set -eu

repo="${BIKEBOOK_REPO:-helopony/bikebook-cli}"
version="${BIKEBOOK_VERSION:-latest}"
install_dir="${BIKEBOOK_INSTALL_DIR:-$HOME/.local/bin}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "bikebook installer requires $1" >&2
    exit 1
  fi
}

need curl

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin|linux) ;;
  msys*|mingw*|cygwin*) os="windows" ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [ "$version" = "latest" ]; then
  tag="$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
else
  tag="$version"
fi

if [ -z "$tag" ]; then
  echo "could not resolve latest bikebook release" >&2
  exit 1
fi

ext="tar.gz"
if [ "$os" = "windows" ]; then
  ext="zip"
  need unzip
else
  need tar
fi

asset="bikebook_${tag}_${os}_${arch}.${ext}"
url="https://github.com/$repo/releases/download/$tag/$asset"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading $url" >&2
curl -fsSL "$url" -o "$tmp/$asset"

mkdir -p "$install_dir"
if [ "$ext" = "zip" ]; then
  unzip -q "$tmp/$asset" -d "$tmp"
  binary="bikebook.exe"
else
  tar -xzf "$tmp/$asset" -C "$tmp"
  binary="bikebook"
fi

if [ ! -f "$tmp/$binary" ]; then
  found="$(find "$tmp" -type f -name "$binary" | head -n1)"
  if [ -z "$found" ]; then
    echo "archive did not contain $binary" >&2
    exit 1
  fi
else
  found="$tmp/$binary"
fi

install -m 0755 "$found" "$install_dir/$binary"
echo "Installed bikebook to $install_dir/$binary" >&2
echo "Run: $install_dir/$binary version" >&2
