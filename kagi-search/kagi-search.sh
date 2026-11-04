#!/usr/bin/env bash
# Wrapper for the kagi-search skill.
#
# Resolves a binary, building from source when Go is available and otherwise
# offering a checksum-verified download from GitHub releases, then execs it.
set -euo pipefail

BIN_NAME="kagi-search"
REPO="Nighthawk42/kagi-skills"

# Resolve symlinks: skills are usually symlinked into an agent's skills
# directory, and we need the real path to find the module root above us.
SOURCE="${BASH_SOURCE[0]}"
while [[ -L "$SOURCE" ]]; do
  TARGET="$(readlink "$SOURCE")"
  if [[ "$TARGET" == /* ]]; then SOURCE="$TARGET"; else
    SOURCE="$(cd -P "$(dirname "$SOURCE")" && pwd)/$TARGET"
  fi
done
BASE_DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
REPO_ROOT="$(cd -P "$BASE_DIR/.." && pwd)"

BIN_DIR="$BASE_DIR/.bin"
BIN="$BIN_DIR/$BIN_NAME"

# A source checkout has the module root one level up. When the skill directory
# was copied somewhere on its own, there is nothing to build from.
HAVE_SOURCE=0
if [[ -f "$REPO_ROOT/go.mod" && -d "$REPO_ROOT/cmd/$BIN_NAME" ]]; then
  HAVE_SOURCE=1
fi

needs_build=0
if [[ ! -x "$BIN" ]]; then
  needs_build=1
elif [[ "$HAVE_SOURCE" -eq 1 ]]; then
  # Rebuild when any Go source or the module file is newer than the binary.
  if [[ -n "$(find "$REPO_ROOT/cmd" "$REPO_ROOT/internal" -name '*.go' -newer "$BIN" -print -quit 2>/dev/null)" ]] \
    || [[ "$REPO_ROOT/go.mod" -nt "$BIN" ]]; then
    needs_build=1
  fi
fi

go_version_ok() {
  local v
  v="$(go env GOVERSION 2>/dev/null || true)"
  [[ -z "$v" ]] && return 1
  v="${v#go}"
  local major="${v%%.*}" rest="${v#*.}" minor
  minor="${rest%%.*}"
  [[ "$major" -gt 1 ]] && return 0
  [[ "$major" -eq 1 && "$minor" -ge 26 ]]
}

sha256_of() {
  # Pick whatever this system actually has. The previous version of this
  # script used only `shasum`, which is absent on most Linux distributions,
  # so checksum verification aborted the install under `set -e`.
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{print $NF}'
  elif command -v certutil >/dev/null 2>&1; then
    certutil -hashfile "$1" SHA256 | sed -n '2p' | tr -d '[:space:]' | tr 'A-Z' 'a-z'
  else
    return 1
  fi
}

have_downloader() {
  command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1
}

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    return 1
  fi
}

fetch_to() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    return 1
  fi
}

if [[ "$needs_build" -eq 1 ]]; then
  mkdir -p "$BIN_DIR"

  if [[ "$HAVE_SOURCE" -eq 1 ]] && command -v go >/dev/null 2>&1; then
    if go_version_ok; then
      echo "Building $BIN_NAME from source..." >&2
      # Inject a version so --version and bug reports identify the build,
      # rather than every source build calling itself "dev".
      VERSION="$(cd "$REPO_ROOT" && git describe --tags --always --dirty 2>/dev/null || true)"
      LDFLAGS="-s -w"
      if [[ -n "$VERSION" ]]; then
        LDFLAGS="$LDFLAGS -X github.com/Nighthawk42/kagi-skills/internal/cli.Version=$VERSION"
      fi
      (cd "$REPO_ROOT" && go build -ldflags "$LDFLAGS" -o "$BIN" "./cmd/$BIN_NAME")
    else
      echo "Warning: Go 1.26+ is required to build; found $(go env GOVERSION 2>/dev/null)." >&2
      echo "Falling back to a pre-built binary." >&2
    fi
  fi

  if [[ ! -x "$BIN" ]]; then
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    EXT=""
    case "$OS" in
      mingw* | msys* | cygwin* | windows*)
        OS="windows"
        # Release artifacts for Windows carry a .exe suffix. Omitting it here
        # is a 404.
        EXT=".exe"
        ;;
    esac
    case "$ARCH" in
      x86_64 | amd64) ARCH="amd64" ;;
      aarch64 | arm64) ARCH="arm64" ;;
    esac

    if ! have_downloader; then
      echo "Error: $BIN_NAME is not installed, and neither curl nor wget is available" >&2
      echo "to download it." >&2
      echo "" >&2
      echo "Install Go 1.26+ and run from a source checkout to build it instead, or" >&2
      echo "download it manually from:" >&2
      echo "  https://github.com/$REPO/releases/latest" >&2
      exit 1
    fi

    TAG=""
    if RELEASE_META="$(fetch "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null)"; then
      TAG="$(printf '%s\n' "$RELEASE_META" | grep -m1 '"tag_name"' | cut -d'"' -f4 || true)"
    fi
    if [[ -z "$TAG" ]]; then
      echo "Error: could not find a published release for $REPO." >&2
      echo "" >&2
      echo "Either the repository has no releases yet, or GitHub could not be reached." >&2
      echo "Install Go 1.26+ and run this from a source checkout to build from source:" >&2
      echo "  git clone https://github.com/$REPO" >&2
      echo "  cd kagi-skills && make build" >&2
      exit 1
    fi

    BINARY="${BIN_NAME}_${TAG}_${OS}_${ARCH}${EXT}"
    URL="https://github.com/$REPO/releases/download/${TAG}/${BINARY}"
    CHECKSUMS_URL="https://github.com/$REPO/releases/download/${TAG}/checksums.txt"

    # Downloading and running an executable deserves consent. When there is no
    # terminal to ask at -- the common case, since these skills are invoked by
    # agents -- refuse rather than silently fetching a binary.
    #
    # Note that testing for /dev/tty with -e is not enough: under Git Bash the
    # path exists but cannot be opened. Probe it in a subshell, so a failure
    # cannot take down this shell, and treat anything other than an explicit
    # yes as a refusal.
    if [[ "${KAGI_SKILLS_ASSUME_YES:-}" != "1" ]]; then
      consent="unasked"
      if (exec 3< /dev/tty) 2>/dev/null; then
        echo "$BIN_NAME is not installed. Download the pre-built binary?" >&2
        echo "  $URL" >&2
        reply=""
        if read -r -p "Download? [Y/n] " reply < /dev/tty; then
          case "${reply:-Y}" in
            [Yy]* | "") consent="yes" ;;
            *) consent="no" ;;
          esac
        fi
      fi

      if [[ "$consent" == "no" ]]; then
        echo "Aborted." >&2
        exit 1
      fi

      if [[ "$consent" != "yes" ]]; then
        echo "Error: $BIN_NAME is not installed and there is no terminal to confirm a download." >&2
        echo "" >&2
        echo "Pick one:" >&2
        echo "  * Install Go 1.26+ and run from a source checkout; this wrapper" >&2
        echo "    then builds from source." >&2
        echo "  * Run this once from an interactive shell to install it." >&2
        echo "  * Set KAGI_SKILLS_ASSUME_YES=1 to allow the download:" >&2
        echo "      $URL" >&2
        exit 1
      fi
    fi

    TMP="$(mktemp "${BIN}.XXXXXX")"
    trap 'rm -f "$TMP"' EXIT

    if ! fetch_to "$URL" "$TMP"; then
      echo "Error: failed to download $URL" >&2
      exit 1
    fi

    if CHECKSUMS="$(fetch "$CHECKSUMS_URL")"; then
      EXPECTED="$(printf '%s\n' "$CHECKSUMS" | grep -F " $BINARY" | awk '{print $1}' | head -n1)"
    else
      EXPECTED=""
    fi

    if [[ -z "$EXPECTED" ]]; then
      echo "Error: no checksum for $BINARY in checksums.txt; refusing to install." >&2
      exit 1
    fi

    if ! ACTUAL="$(sha256_of "$TMP")"; then
      echo "Error: no SHA-256 tool available (sha256sum, shasum, openssl or certutil)." >&2
      echo "Refusing to install an unverified binary." >&2
      exit 1
    fi

    if [[ "$ACTUAL" != "$EXPECTED" ]]; then
      echo "Error: checksum mismatch for $BINARY." >&2
      echo "  Expected: $EXPECTED" >&2
      echo "  Actual:   $ACTUAL" >&2
      exit 1
    fi

    chmod +x "$TMP"
    mv "$TMP" "$BIN"
    trap - EXIT
    echo "Installed $BIN_NAME $TAG (checksum verified)." >&2
  fi
fi

exec "$BIN" "$@"
