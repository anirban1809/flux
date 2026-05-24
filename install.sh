#!/usr/bin/env bash
set -euo pipefail

REPO="anirban1809/flux"
BIN_NAME="flux"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

err() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '==> %s\n' "$*"; }

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v uname >/dev/null 2>&1 || err "uname is required"

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Linux)  os_tag="linux"  ;;
  Darwin) os_tag="darwin" ;;
  *) err "unsupported OS: $os" ;;
esac

case "$arch" in
  x86_64|amd64) arch_tag="amd64" ;;
  arm64|aarch64) arch_tag="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

if [ "$os_tag" = "linux" ] && [ "$arch_tag" = "arm64" ]; then
  err "no linux-arm64 release published for $REPO"
fi

asset="${BIN_NAME}-${os_tag}-${arch_tag}"

info "Resolving latest release for $REPO"
api_url="https://api.github.com/repos/${REPO}/releases/latest"
auth=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

release_json="$(curl -fsSL "${auth[@]}" -H "Accept: application/vnd.github+json" "$api_url")" \
  || err "failed to query GitHub API"

tag="$(printf '%s' "$release_json" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
[ -n "$tag" ] || err "could not parse tag_name from release"

download_url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
checksum_url="https://github.com/${REPO}/releases/download/${tag}/SHA256SUMS"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

info "Downloading $asset ($tag)"
curl -fL --progress-bar -o "$tmpdir/$asset" "$download_url" \
  || err "download failed: $download_url"

info "Fetching SHA256SUMS"
if curl -fsSL -o "$tmpdir/SHA256SUMS" "$checksum_url"; then
  if command -v shasum >/dev/null 2>&1; then
    sha_cmd=(shasum -a 256)
  elif command -v sha256sum >/dev/null 2>&1; then
    sha_cmd=(sha256sum)
  else
    sha_cmd=()
  fi

  if [ "${#sha_cmd[@]}" -gt 0 ]; then
    expected="$(grep " ${asset}\$" "$tmpdir/SHA256SUMS" | awk '{print $1}' || true)"
    if [ -n "$expected" ]; then
      actual="$("${sha_cmd[@]}" "$tmpdir/$asset" | awk '{print $1}')"
      [ "$expected" = "$actual" ] || err "checksum mismatch (expected $expected, got $actual)"
      info "Checksum verified"
    fi
  fi
else
  info "SHA256SUMS not found, skipping verification"
fi

chmod +x "$tmpdir/$asset"

dest="$INSTALL_DIR/$BIN_NAME"
info "Installing to $dest"

if [ -w "$INSTALL_DIR" ] || { [ ! -e "$INSTALL_DIR" ] && mkdir -p "$INSTALL_DIR" 2>/dev/null; }; then
  mv "$tmpdir/$asset" "$dest"
else
  command -v sudo >/dev/null 2>&1 || err "$INSTALL_DIR is not writable and sudo is unavailable"
  sudo mv "$tmpdir/$asset" "$dest"
fi

info "Installed $BIN_NAME $tag to $dest"
"$dest" --version 2>/dev/null || true
