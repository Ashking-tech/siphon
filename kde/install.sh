#!/usr/bin/env bash
#
# Install the Siphon Dolphin ServiceMenus and the siphon-gui-kde wrapper into
# a user-local prefix. This is intentionally a small, auditable script; a
# distribution package should install the same files under /usr.
set -euo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

PREFIX="${PREFIX:-$HOME/.local}"
BINDIR=""
DATADIR=""
PLASMA="both"
INSTALL_BINARY=1
REBUILD_CACHE=1
UNINSTALL=0

usage() {
	cat <<'EOF'
Usage: kde/install.sh [options]

Install Siphon's Dolphin ServiceMenus and siphon-gui-kde for the current user.

Options:
  --prefix <dir>     Install prefix (default: $HOME/.local)
  --bindir <dir>     Binary directory (default: <prefix>/bin)
  --datadir <dir>    Data directory (default: <prefix>/share)
  --plasma <which>   auto, 5, 6, or both (default: both)
  --no-binary        Do not install bin/siphon next to the wrapper
  --no-cache         Do not run kbuildsycoca6 after installing
  --uninstall        Remove the wrapper and ServiceMenus
  -h, --help         Show this help

The wrapper and binary are installed under the prefix, so the ServiceMenu can
run without depending on the caller's PATH. If --no-binary is used, siphon must
be in PATH or SIPHON_BIN must be set when the menu action runs.
EOF
}

die() {
	printf 'kde/install.sh: %s\n' "$*" >&2
	exit 2
}

while [[ $# -gt 0 ]]; do
	case $1 in
	--prefix)
		[[ $# -ge 2 ]] || die "--prefix requires a value"
		PREFIX=$2
		shift 2
		;;
	--bindir)
		[[ $# -ge 2 ]] || die "--bindir requires a value"
		BINDIR=$2
		shift 2
		;;
	--datadir)
		[[ $# -ge 2 ]] || die "--datadir requires a value"
		DATADIR=$2
		shift 2
		;;
	--plasma)
		[[ $# -ge 2 ]] || die "--plasma requires a value"
		PLASMA=$2
		shift 2
		;;
	--no-binary)
		INSTALL_BINARY=0
		shift
		;;
	--no-cache)
		REBUILD_CACHE=0
		shift
		;;
	--uninstall)
		UNINSTALL=1
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

case $PLASMA in
auto)
	if command -v plasmashell >/dev/null 2>&1; then
		version="$(plasmashell --version 2>/dev/null || true)"
		case $version in
		*" 6."*) PLASMA=6 ;;
		*" 5."*) PLASMA=5 ;;
		*) PLASMA=both ;;
		esac
	elif command -v kbuildsycoca6 >/dev/null 2>&1; then
		PLASMA=6
	elif command -v kbuildsycoca5 >/dev/null 2>&1; then
		PLASMA=5
	else
		PLASMA=both
	fi
	;;
5 | 6 | both) ;;
*) die "--plasma must be auto, 5, 6, or both" ;;
esac

BINDIR="${BINDIR:-$PREFIX/bin}"
DATADIR="${DATADIR:-$PREFIX/share}"
# The installed .desktop files use an absolute wrapper path: Dolphin is often
# launched with a session PATH that does not include ~/.local/bin.
BINDIR="$(realpath -m -- "$BINDIR")"
DATADIR="$(realpath -m -- "$DATADIR")"

declare -a menu_dirs=()
case $PLASMA in
5) menu_dirs+=("$DATADIR/kservices5/ServiceMenus") ;;
6) menu_dirs+=("$DATADIR/kio/servicemenus") ;;
both)
	menu_dirs+=("$DATADIR/kio/servicemenus")
	menu_dirs+=("$DATADIR/kservices5/ServiceMenus")
	;;
esac

wrapper_target="$BINDIR/siphon-gui-kde"

desktop_exec_path() {
	local path=$1
	path=${path//\\/\\\\}
	path=${path//\"/\\\"}
	if [[ $path == *[[:space:]]* ]]; then
		printf '"%s"' "$path"
	else
		printf '%s' "$path"
	fi
}

rebuild_cache() {
	((REBUILD_CACHE == 1)) || return 0
	if command -v kbuildsycoca6 >/dev/null 2>&1; then
		kbuildsycoca6 --noincremental >/dev/null 2>&1 || true
	elif command -v kbuildsycoca5 >/dev/null 2>&1; then
		kbuildsycoca5 --noincremental >/dev/null 2>&1 || true
	fi
}

if ((UNINSTALL == 1)); then
	for dir in "${menu_dirs[@]}"; do
		rm -f -- "$dir"/siphon-*.desktop
	done
	rm -f -- "$wrapper_target"
	# Leave $BINDIR/siphon alone: it may be a package-managed binary. Use the
	# package manager or remove it manually if this installer placed it.
	rebuild_cache
	printf 'Removed Siphon ServiceMenus and %s\n' "$wrapper_target"
	exit 0
fi

[[ -f $SCRIPT_DIR/siphon-gui-kde ]] || die "missing wrapper: $SCRIPT_DIR/siphon-gui-kde"
[[ -d $SCRIPT_DIR/servicemenus ]] || die "missing servicemenus directory"

mkdir -p -- "$BINDIR" "${menu_dirs[@]}"
install -m 0755 -- "$SCRIPT_DIR/siphon-gui-kde" "$wrapper_target"

exec_path="$(desktop_exec_path "$wrapper_target")"
for src in "$SCRIPT_DIR"/servicemenus/siphon-*.desktop; do
	[[ -f $src ]] || die "no ServiceMenu files found"
	for dir in "${menu_dirs[@]}"; do
		tmp="$dir/.siphon-install.$$"
		while IFS= read -r line || [[ -n $line ]]; do
			if [[ $line == "Exec=siphon-gui-kde "* ]]; then
				printf 'Exec=%s %s\n' "$exec_path" "${line#Exec=siphon-gui-kde }"
			else
				printf '%s\n' "$line"
			fi
		done <"$src" >"$tmp"
		# KDE refuses user-owned ServiceMenu files without the execute bit.
		install -m 0755 -- "$tmp" "$dir/$(basename -- "$src")"
		rm -f -- "$tmp"
	done
done

if ((INSTALL_BINARY == 1)); then
	source_binary=""
	if [[ -n ${SIPHON_BIN:-} && -x ${SIPHON_BIN:-} ]]; then
		source_binary=$SIPHON_BIN
	elif [[ -x $REPO_DIR/bin/siphon ]]; then
		source_binary="$REPO_DIR/bin/siphon"
	elif command -v siphon >/dev/null 2>&1; then
		source_binary="$(command -v siphon)"
	fi
	if [[ -n $source_binary ]]; then
		install -m 0755 -- "$source_binary" "$BINDIR/siphon"
		printf 'Installed %s\n' "$BINDIR/siphon"
	else
		printf 'warning: siphon binary not found; the wrapper will use PATH or SIPHON_BIN\n' >&2
	fi
fi

rebuild_cache
printf 'Installed %s\n' "$wrapper_target"
for dir in "${menu_dirs[@]}"; do
	printf 'Installed ServiceMenus in %s\n' "$dir"
done
printf 'Open a new Dolphin window (or run kbuildsycoca6) to see the Siphon submenu.\n'
