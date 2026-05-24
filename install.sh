#!/usr/bin/env bash
set -euo pipefail

REPO="anirban1809/flux"
BIN_NAME="flux"
FLUX_HOME="${FLUX_HOME:-$HOME/.flux}"
INSTALL_DIR="${INSTALL_DIR:-$FLUX_HOME/bin}"

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
if [ -n "${GITHUB_TOKEN:-}" ]; then
  release_json="$(curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" -H "Accept: application/vnd.github+json" "$api_url")" \
    || err "failed to query GitHub API"
else
  release_json="$(curl -fsSL -H "Accept: application/vnd.github+json" "$api_url")" \
    || err "failed to query GitHub API"
fi

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

mkdir -p "$INSTALL_DIR" || err "failed to create $INSTALL_DIR"

dest="$INSTALL_DIR/$BIN_NAME"
info "Installing to $dest"
mv "$tmpdir/$asset" "$dest"

info "Installed $BIN_NAME $tag to $dest"

add_path_line() {
  local rc="$1"
  local line="$2"
  [ -f "$rc" ] || return 0
  if ! grep -Fq "$line" "$rc" 2>/dev/null; then
    printf '\n# Added by flux installer\n%s\n' "$line" >> "$rc"
    info "Updated $rc"
  fi
}

path_line="export PATH=\"$INSTALL_DIR:\$PATH\""
updated=0

case "${SHELL:-}" in
  */zsh)
    add_path_line "$HOME/.zshrc" "$path_line" && updated=1
    ;;
  */bash)
    if [ -f "$HOME/.bashrc" ]; then
      add_path_line "$HOME/.bashrc" "$path_line" && updated=1
    fi
    if [ -f "$HOME/.bash_profile" ] || [ "$(uname -s)" = "Darwin" ]; then
      add_path_line "$HOME/.bash_profile" "$path_line" && updated=1
    fi
    ;;
  */fish)
    fish_config="$HOME/.config/fish/config.fish"
    if [ -f "$fish_config" ]; then
      fish_line="set -gx PATH $INSTALL_DIR \$PATH"
      if ! grep -Fq "$fish_line" "$fish_config" 2>/dev/null; then
        printf '\n# Added by flux installer\n%s\n' "$fish_line" >> "$fish_config"
        info "Updated $fish_config"
        updated=1
      fi
    fi
    ;;
esac

case ":${PATH}:" in
  *":$INSTALL_DIR:"*)
    ;;
  *)
    if [ "$updated" -eq 1 ]; then
      info "Restart your shell or 'source' your rc file to pick up the new PATH"
    else
      info "Add this to your shell rc to use $BIN_NAME from anywhere:"
      printf '    %s\n' "$path_line"
    fi
    ;;
esac

"$dest" --version 2>/dev/null || true
