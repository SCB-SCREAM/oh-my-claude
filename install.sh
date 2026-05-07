#!/bin/sh
# install.sh — install the latest (or pinned) omc release into ~/.local/bin
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/SCB-SCREAM/oh-my-claude/main/install.sh | sh
#
# Security-conscious form (recommended):
#   curl -fsSL https://raw.githubusercontent.com/SCB-SCREAM/oh-my-claude/main/install.sh -o install.sh
#   less install.sh   # read it
#   sh install.sh
#
# Environment overrides:
#   OMC_VERSION       Tag to install (default: latest GitHub Release).
#                     Accepts "v0.1.0" or "0.1.0".
#   OMC_INSTALL_DIR   Where to put the omc binary (default: $HOME/.local/bin).

set -eu

OWNER="SCB-SCREAM"
REPO="oh-my-claude"
BIN_NAME="omc"

# ---------------------------------------------------------------------- helpers

log()  { printf '%s\n' "$*"; }
err()  { printf 'error: %s\n' "$*" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

# Pick a downloader. curl preferred (better progress, ubiquitous on macOS).
download_to() {
    url="$1"; dest="$2"
    if have curl; then
        curl -fsSL --proto '=https' --tlsv1.2 -o "$dest" "$url"
    elif have wget; then
        wget -qO "$dest" "$url"
    else
        err "neither curl nor wget is installed"
    fi
}

# Pick a sha256 verifier. macOS lacks sha256sum but has shasum -a 256.
sha256_check() {
    file="$1"; expected="$2"
    if have sha256sum; then
        printf '%s  %s\n' "$expected" "$file" | sha256sum -c - >/dev/null
    elif have shasum; then
        printf '%s  %s\n' "$expected" "$file" | shasum -a 256 -c - >/dev/null
    else
        err "neither sha256sum nor shasum is installed"
    fi
}

# --------------------------------------------------------------- detect target

case "$(uname -s)" in
    Linux*)  OS="linux"  ;;
    Darwin*) OS="darwin" ;;
    *)       err "unsupported OS: $(uname -s) (omc supports linux + darwin via this script; use scoop on Windows)" ;;
esac

case "$(uname -m)" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             err "unsupported architecture: $(uname -m)" ;;
esac

# ----------------------------------------------------------------- resolve tag

VERSION="${OMC_VERSION:-}"
if [ -z "$VERSION" ]; then
    log "Resolving latest release..."
    api="https://api.github.com/repos/$OWNER/$REPO/releases/latest"
    tmp="$(mktemp)"
    download_to "$api" "$tmp" || err "could not reach $api"
    # Extract tag_name without depending on jq.
    VERSION="$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp" | head -1)"
    rm -f "$tmp"
fi
[ -n "$VERSION" ] || err "could not resolve a release version"

# Normalize: archive names use the bare version, GitHub Release tags use 'v'.
TAG="$VERSION"
case "$TAG" in v*) ;; *) TAG="v$TAG" ;; esac
BARE="${TAG#v}"

ARCHIVE="${BIN_NAME}_${BARE}_${OS}_${ARCH}.tar.gz"
ARCHIVE_URL="https://github.com/$OWNER/$REPO/releases/download/$TAG/$ARCHIVE"
CHECKSUMS_URL="https://github.com/$OWNER/$REPO/releases/download/$TAG/checksums.txt"

# --------------------------------------------------------------------- workdir

TMP="$(mktemp -d 2>/dev/null || mktemp -d -t omc-install)"
trap 'rm -rf "$TMP"' EXIT INT HUP TERM

# ---------------------------------------------------------- download + verify

log "Downloading $ARCHIVE ($TAG, $OS/$ARCH)..."
download_to "$ARCHIVE_URL"   "$TMP/$ARCHIVE"
download_to "$CHECKSUMS_URL" "$TMP/checksums.txt"

expected="$(awk -v f="$ARCHIVE" '$2 == f {print $1}' "$TMP/checksums.txt")"
[ -n "$expected" ] || err "no checksum entry for $ARCHIVE in checksums.txt"

log "Verifying SHA256..."
(cd "$TMP" && sha256_check "$ARCHIVE" "$expected") || err "checksum mismatch — refusing to install"

# --------------------------------------------------------------------- install

log "Extracting..."
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
[ -f "$TMP/$BIN_NAME" ] || err "archive did not contain '$BIN_NAME'"

INSTALL_DIR="${OMC_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$INSTALL_DIR"

mv "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

log ""
log "✓ installed $BIN_NAME $TAG → $INSTALL_DIR/$BIN_NAME"

# ---------------------------------------------------------------- PATH advice

case ":${PATH-}:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        log ""
        log "NOTE: $INSTALL_DIR is not on your \$PATH."
        log "      Add this line to your shell's rc file (~/.bashrc, ~/.zshrc, …):"
        log ""
        log "          export PATH=\"$INSTALL_DIR:\$PATH\""
        ;;
esac

log ""
"$INSTALL_DIR/$BIN_NAME" --version || true
