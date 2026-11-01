#!/bin/sh
#
# Install Siphon from a published GitHub release.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Ashking-tech/siphon/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --no-kde
#   curl -fsSL .../install.sh | sh -s -- --version v0.2.1 --prefix "$HOME/.local"
#
set -eu

REPO="Ashking-tech/siphon"
VERSION="${SIPHON_VERSION:-}"
PREFIX="${PREFIX:-}"
PLASMA="both"
NO_KDE=0
NO_CACHE=0

usage() {
	cat <<'EOF'
Install Siphon from a published GitHub release.

Usage:
  install.sh [options]

Options:
  --version <tag>    Release tag to install (default: latest)
  --prefix <dir>     Install prefix (default: $HOME/.local)
  --plasma <which>   KDE menus: auto, 5, 6, or both (default: both)
  --no-kde           Install the CLI only; skip the Dolphin ServiceMenu
  --no-cache         Do not run kbuildsycoca6 after installing KDE menus
  -h, --help         Show this help

Environment:
  SIPHON_VERSION     Same as --version
  PREFIX             Same as --prefix

Examples:
  curl -fsSL https://raw.githubusercontent.com/Ashking-tech/siphon/main/install.sh | sh
  curl -fsSL .../install.sh | sh -s -- --no-kde
  curl -fsSL .../install.sh | sh -s -- --version v0.2.1 --prefix "$HOME/.local"
EOF
}

die() {
	printf 'siphon-install: %s\n' "$*" >&2
	exit 1
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || die "--version requires a value"
		VERSION=$2
		shift 2
		;;
	--prefix)
		[ "$#" -ge 2 ] || die "--prefix requires a value"
		PREFIX=$2
		shift 2
		;;
	--plasma)
		[ "$#" -ge 2 ] || die "--plasma requires a value"
		PLASMA=$2
		shift 2
		;;
	--no-kde)
		NO_KDE=1
		shift
		;;
	--no-cache)
		NO_CACHE=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "unknown option '$1' (try --help)"
		;;
	esac
done

if [ -z "$PREFIX" ]; then
	[ -n "${HOME:-}" ] || die "HOME is not set; pass --prefix"
	PREFIX="$HOME/.local"
fi
case "$PREFIX" in
/*) ;;
*) PREFIX="$(pwd)/$PREFIX" ;;
esac
case "$PREFIX" in
/) ;;
*) PREFIX=${PREFIX%/} ;;
esac

for cmd in curl tar sha256sum uname install grep sed head mktemp; do
	command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
done

[ "$(uname -s)" = "Linux" ] || die "Siphon supports Linux only"

machine=$(uname -m)
case "$machine" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "unsupported architecture '$machine' (supported: x86_64, aarch64)" ;;
esac

if [ -z "$VERSION" ]; then
	latest=$(curl -fsSL --retry 3 --proto '=https' --tlsv1.2 \
		"https://api.github.com/repos/$REPO/releases/latest") ||
		die "could not query the latest release; pass --version"
	VERSION=$(printf '%s\n' "$latest" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1)
	[ -n "$VERSION" ] || die "could not determine the latest release; pass --version"
fi
case "$VERSION" in
v*) ;;
*) VERSION="v$VERSION" ;;
esac
ver=${VERSION#v}
case "$ver" in
'' | *[!0-9A-Za-z._-]*) die "invalid version '$VERSION'" ;;
esac

tmp=$(mktemp -d "${TMPDIR:-/tmp}/siphon-install.XXXXXX") ||
	die "could not create a temporary directory"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

archive="siphon_${ver}_linux_${arch}.tar.gz"
checksums="siphon_${ver}_checksums.txt"
base="https://github.com/$REPO/releases/download/$VERSION"

printf 'Downloading Siphon %s for linux/%s...\n' "$VERSION" "$arch"
curl -fsSL --retry 3 --proto '=https' --tlsv1.2 \
	"$base/$archive" -o "$tmp/$archive" ||
	die "download failed: $base/$archive"
curl -fsSL --retry 3 --proto '=https' --tlsv1.2 \
	"$base/$checksums" -o "$tmp/$checksums" ||
	die "download failed: $base/$checksums"

grep -Fq "  $archive" "$tmp/$checksums" ||
	die "checksums file does not list $archive"

(
	cd "$tmp"
	sha256sum --check --ignore-missing "$checksums"
) || die "checksum verification failed"

mkdir -p "$tmp/extract"
tar -xzf "$tmp/$archive" -C "$tmp/extract"
[ -x "$tmp/extract/siphon" ] || die "archive did not contain the siphon binary"

if [ "$NO_KDE" -eq 1 ]; then
	mkdir -p "$PREFIX/bin"
	install -m 0755 "$tmp/extract/siphon" "$PREFIX/bin/siphon"
	printf 'Installed %s\n' "$PREFIX/bin/siphon"
else
	set -- --prefix "$PREFIX" --plasma "$PLASMA"
	if [ "$NO_CACHE" -eq 1 ]; then
		set -- "$@" --no-cache
	fi
	"$tmp/extract/kde/install.sh" "$@"
fi

if [ -x "$PREFIX/bin/siphon" ]; then
	"$PREFIX/bin/siphon" version
	case ":${PATH:-}:" in
	*":$PREFIX/bin:"*) ;;
	*) printf 'Add %s to PATH to run siphon directly.\n' "$PREFIX/bin" ;;
	esac
fi

if [ "$NO_KDE" -eq 0 ]; then
	printf 'Restart Dolphin to see the Siphon ServiceMenu.\n'
fi
