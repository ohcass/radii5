#!/usr/bin/env bash
set -euo pipefail

BOLD="\033[1m"
CYAN="\033[36m"
GREEN="\033[32m"
RED="\033[31m"
RESET="\033[0m"

echo ""
printf "  ${CYAN}→${RESET} ${BOLD}radii5 installer${RESET}\n"
echo ""

# ── detect platform ──────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH_F="amd64" ;;
  aarch64|arm64) ARCH_F="arm64" ;;
  *)
    printf "  ${RED}✗ unsupported arch: $ARCH${RESET}\n"
    exit 1
    ;;
esac

case "$OS" in
  linux)   OS_F="linux" ;;
  darwin)  OS_F="macos" ;;
  *)
    printf "  ${RED}✗ unsupported OS: $OS${RESET}\n"
    exit 1
    ;;
esac

BIN_DIR="$HOME/.radii5/bin"
mkdir -p "$BIN_DIR"

# ── helper: download with progress ──────────────────────────────
download() {
  local url="$1" dest="$2" desc="$3"
  printf "  ${CYAN}↓${RESET}  downloading ${desc}...\n"
  if command -v wget &>/dev/null; then
    wget -q --show-progress -O "$dest" "$url"
  else
    curl -#fSL -o "$dest" "$url"
  fi
}

cleanup() {
  local d="$1"
  rm -rf "$d"
}

# ── 1. yt-dlp ────────────────────────────────────────────────────
YTDLP_URL="https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
YTDLP_DEST="$BIN_DIR/yt-dlp"

if [ -x "$YTDLP_DEST" ]; then
  printf "  ${GREEN}✓${RESET}  yt-dlp already installed\n"
else
  download "$YTDLP_URL" "$YTDLP_DEST" "yt-dlp"
  chmod +x "$YTDLP_DEST"
  printf "  ${GREEN}✓${RESET}  yt-dlp installed\n"
fi

# ── 2. ffmpeg ────────────────────────────────────────────────────
FFMPEG_DEST="$BIN_DIR/ffmpeg"

install_ffmpeg_linux() {
  local url arch_url
  case "$ARCH_F" in
    amd64) arch_url="amd64" ;;
    arm64) arch_url="arm64" ;;
  esac
  url="https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-${arch_url}-static.tar.xz"
  local tmpdir
  tmpdir=$(mktemp -d)
  download "$url" "$tmpdir/ffmpeg.tar.xz" "ffmpeg"
  tar -xJf "$tmpdir/ffmpeg.tar.xz" -C "$tmpdir"
  find "$tmpdir" -name "ffmpeg" -type f -exec cp {} "$FFMPEG_DEST" \;
  chmod +x "$FFMPEG_DEST"
  rm -rf "$tmpdir"
}

install_ffmpeg_macos() {
  local url
  case "$ARCH_F" in
    amd64) url="https://evermeet.cx/ffmpeg/get/ffmpeg.zip" ;;
    arm64) url="https://evermeet.cx/ffmpeg/get/ffmpeg.zip" ;;
  esac
  local tmpdir
  tmpdir=$(mktemp -d)
  download "$url" "$tmpdir/ffmpeg.zip" "ffmpeg"
  unzip -q -o "$tmpdir/ffmpeg.zip" -d "$tmpdir"
  cp "$tmpdir/ffmpeg" "$FFMPEG_DEST"
  chmod +x "$FFMPEG_DEST"
  rm -rf "$tmpdir"
}

if [ -x "$FFMPEG_DEST" ]; then
  printf "  ${GREEN}✓${RESET}  ffmpeg already installed\n"
else
  case "$OS_F" in
    linux) install_ffmpeg_linux ;;
    macos) install_ffmpeg_macos ;;
  esac
  printf "  ${GREEN}✓${RESET}  ffmpeg installed\n"
fi

# ── 3. radii5 binary ────────────────────────────────────────────
RADII5_DEST="$BIN_DIR/radii5"

if [ -x "$RADII5_DEST" ]; then
  latest=$(curl -fsSL "https://api.github.com/repos/ohcass/radii5/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
  installed_hash=$(sha256sum "$RADII5_DEST" | cut -d' ' -f1)
  expected_hash=$(curl -fsSL "https://github.com/ohcass/radii5/releases/download/$latest/checksums.txt" | grep "radii5-${OS_F}-${ARCH_F}" | cut -d' ' -f1)
  if [ "$installed_hash" = "$expected_hash" ]; then
    printf "  ${GREEN}✓${RESET}  radii5 up to date\n"
  else
    printf "  ${CYAN}↻${RESET}  updating radii5...\n"

    # Try bsdiff patch first for incremental update
    patched=false
    installed_version=$("$RADII5_DEST" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^ ]*' || echo "")
    if [ -n "$installed_version" ]; then
      patch_name="patch_${OS_F}-${ARCH_F}_${installed_version}_${latest}.bspatch"
      patch_url="https://github.com/ohcass/radii5/releases/download/$latest/$patch_name"

      TMPDIR_PATCH=$(mktemp -d)
      patch_dest="$TMPDIR_PATCH/update.bspatch"
      new_bin="$TMPDIR_PATCH/radii5-new"

      if download "$patch_url" "$patch_dest" "patch" 2>/dev/null; then
        if "$RADII5_DEST" bspatch "$RADII5_DEST" "$new_bin" "$patch_dest" 2>/dev/null; then
          new_hash=$(sha256sum "$new_bin" | cut -d' ' -f1)
          if [ "$new_hash" = "$expected_hash" ]; then
            mv "$new_bin" "$RADII5_DEST"
            chmod +x "$RADII5_DEST"
            patched=true
            printf "  ${GREEN}✓${RESET}  radii5 updated (patch)\n"
          fi
        fi
      fi

      rm -rf "$TMPDIR_PATCH"
    fi

    if [ "$patched" = false ]; then
      download "https://github.com/ohcass/radii5/releases/download/$latest/radii5-${OS_F}-${ARCH_F}" "$RADII5_DEST" "radii5"
      chmod +x "$RADII5_DEST"
      printf "  ${GREEN}✓${RESET}  radii5 updated (full)\n"
    fi
  fi
elif command -v go &>/dev/null; then
  printf "  ${CYAN}↻${RESET}  building radii5 from source...\n"
  SELF_DIR="$(cd "$(dirname "$0")/.." && pwd)"
  if [ -f "$SELF_DIR/go.mod" ]; then
    cd "$SELF_DIR"
    go build -o "$RADII5_DEST" ./cmd/radii5/
    printf "  ${GREEN}✓${RESET}  radii5 built\n"
  else
    printf "  ${CYAN}↻${RESET}  cloning from github.com/ohcass/radii5...\n"
    TMPDIR=$(mktemp -d)
    git clone --depth=1 https://github.com/ohcass/radii5.git "$TMPDIR"
    cd "$TMPDIR"
    go build -o "$RADII5_DEST" ./cmd/radii5/
    rm -rf "$TMPDIR"
    printf "  ${GREEN}✓${RESET}  radii5 built from source\n"
  fi
else
  printf "  ${CYAN}↻${RESET}  Go not found — "
  printf "install Go or download the radii5 binary manually\n"
fi

# ── done ──────────────────────────────────────────────────────────
echo ""
printf "  ${GREEN}${BOLD}✓  installed to ~/.radii5/bin/${RESET}\n"
echo ""

# Check if ~/.radii5/bin is in PATH
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    printf "  ${CYAN}→${RESET}  Add to your PATH:\n"
    printf "        export PATH=\"\$HOME/.radii5/bin:\$PATH\"\n"
    echo ""
    case "$SHELL" in
      *zsh)  printf "        echo 'export PATH=\"\$HOME/.radii5/bin:\$PATH\"' >> ~/.zshrc\n" ;;
      *bash) printf "        echo 'export PATH=\"\$HOME/.radii5/bin:\$PATH\"' >> ~/.bashrc\n" ;;
    esac
    echo ""
    ;;
esac

printf "  ${CYAN}→${RESET}  Run: radii5 <url>\n"
echo ""
