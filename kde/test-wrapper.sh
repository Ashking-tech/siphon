#!/usr/bin/env bash
#
# Mock-driven acceptance tests for the Dolphin wrapper and installer.
# These do not open GUI windows and do not touch the real user configuration.
set -euo pipefail

readonly ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WRAPPER="$ROOT/kde/siphon-gui-kde"
readonly INSTALLER="$ROOT/kde/install.sh"
readonly TMP="$(mktemp -d "${TMPDIR:-/tmp}/siphon-kde-test.XXXXXX")"
readonly MOCK="$TMP/bin"
readonly MOCK_NO_QDBUS="$TMP/bin-no-qdbus"

cleanup() {
	rm -rf -- "$TMP"
}
trap cleanup EXIT

mkdir -p "$MOCK" "$MOCK_NO_QDBUS"
export LOG="$TMP/calls.log"

cat >"$MOCK/siphon" <<'MOCK_SIPHON'
#!/usr/bin/env bash
set -euo pipefail
{
	printf 'siphon'
	for arg in "$@"; do printf ' %s' "$arg"; done
	printf '\n'
} >>"$LOG"

cmd=$1
shift
src=$1

has_flag() {
	local want=$1
	shift
	local arg
	for arg in "$@"; do
		[[ $arg == "$want" ]] && return 0
	done
	return 1
}

fail_json() {
	printf '{"ok":false,"code":"%s","message":"%s","source_untouched":true}\n' "$1" "$2"
	exit 2
}

case $cmd in
inspect)
	case $(basename -- "$src") in
	refuse.mkv)
		fail_json E_CODEC_INCOMPATIBLE "mock codec refusal"
		;;
	meta.wav)
		if has_flag --drop-metadata "$@"; then
			printf '{"ok":true,"plan":{"mode":"safe","warnings":[],"has_alpha":false},"source_untouched":true}\n'
		else
			fail_json E_METADATA_DROP "mock metadata refusal"
		fi
		;;
	alpha.png)
		if has_flag --bg "$@"; then
			printf '{"ok":true,"plan":{"mode":"safe","warnings":[],"has_alpha":true},"source_untouched":true}\n'
		else
			fail_json E_ALPHA_UNSUPPORTED "mock alpha refusal"
		fi
		;;
	target.png)
		if has_flag --force "$@"; then
			printf '{"ok":true,"plan":{"mode":"safe","warnings":[],"has_alpha":false},"source_untouched":true}\n'
		else
			fail_json E_TARGET_EXISTS "mock target exists"
		fi
		;;
	reclaim.wav)
		printf '{"ok":true,"plan":{"mode":"reclaim","crash_cost":"mock crash cost","warnings":[],"has_alpha":false},"source_untouched":true}\n'
		;;
	*)
		printf '{"ok":true,"plan":{"mode":"relocate","warnings":[],"has_alpha":false},"source_untouched":true}\n'
		;;
	esac
	;;
convert)
	if [[ $(basename -- "$src") == fail.mkv ]]; then
		printf '{"phase":"encode","percent":50}\n' >&2
		printf '{"ok":false,"code":"E_TEST","message":"mock conversion failure","source_untouched":false}\n'
		exit 1
	fi
	if [[ $(basename -- "$src") == late-meta.mkv ]] && ! has_flag --drop-metadata "$@"; then
		printf '{"phase":"encode","percent":50}\n' >&2
		printf '{"ok":false,"code":"E_METADATA_DROP","message":"mock late metadata refusal","source_untouched":true}\n'
		exit 2
	fi
	printf '{"phase":"probe","percent":1,"message":"segment 1/1"}\n' >&2
	printf '{"phase":"encode","percent":55}\n' >&2
	printf '{"phase":"verify","percent":90}\n' >&2
	printf '{"ok":true,"source_untouched":false}\n'
	;;
*)
	printf 'mock siphon: unknown command %s\n' "$cmd" >&2
	exit 2
	;;
esac
MOCK_SIPHON

cat >"$MOCK/kdialog" <<'MOCK_KDIALOG'
#!/usr/bin/env bash
set -euo pipefail
{
	printf 'kdialog'
	for arg in "$@"; do printf ' %s' "$arg"; done
	printf '\n'
} >>"$LOG"
case " $* " in
*" --warningcontinuecancel "*)
	exit "${KD_WARN_RC:-0}"
	;;
*" --inputbox "*)
	printf '%s\n' "${KD_INPUT:-white}"
	;;
*" --progressbar "*)
	printf 'org.test.kdialog /org/test/kdialog\n'
	;;
*" --passivepopup "* | *" --error "*)
	;;
esac
MOCK_KDIALOG

cat >"$MOCK/qdbus6" <<'MOCK_QDBUS'
#!/usr/bin/env bash
set -euo pipefail
{
	printf 'qdbus6'
	for arg in "$@"; do printf ' %s' "$arg"; done
	printf '\n'
} >>"$LOG"
if [[ ${QDBUS_PROPERTY_ONLY:-0} == 1 ]]; then
	case " $* " in
	*" org.kde.kdialog.ProgressDialog.setValue "*)
		exit 1
		;;
	*" org.freedesktop.DBus.Properties.Set "*)
		exit 0
		;;
	esac
fi
MOCK_QDBUS

chmod 0755 "$MOCK/siphon" "$MOCK/kdialog" "$MOCK/qdbus6"
cp "$MOCK/kdialog" "$MOCK_NO_QDBUS/kdialog"
cp "$MOCK/siphon" "$MOCK_NO_QDBUS/siphon"
chmod 0755 "$MOCK_NO_QDBUS/kdialog" "$MOCK_NO_QDBUS/siphon"
# The fallback test deliberately uses a PATH with no qdbus at all. Keep only
# the commands the wrapper and mocks need; this avoids picking up a distro
# qdbus-qt6 from /usr/bin.
for tool in bash basename cat dirname jq python3 mktemp mkfifo rm; do
	if command -v "$tool" >/dev/null 2>&1; then
		ln -s "$(command -v "$tool")" "$MOCK_NO_QDBUS/$tool"
	fi
done

run_wrapper() {
	: >"$LOG"
	set +e
	PATH="$MOCK:/usr/bin:/bin" SIPHON_BIN="$MOCK/siphon" \
		"$WRAPPER" "$@" >"$TMP/stdout" 2>"$TMP/stderr"
	status=$?
	set -e
}

expect_status() {
	local want=$1
	if [[ $status -ne $want ]]; then
		printf 'FAIL: status %s, want %s\n' "$status" "$want" >&2
		printf '%s\n' '--- stdout ---' >&2
		cat "$TMP/stdout" >&2
		printf '%s\n' '--- stderr ---' >&2
		cat "$TMP/stderr" >&2
		printf '%s\n' '--- calls ---' >&2
		cat "$LOG" >&2
		exit 1
	fi
}

expect_log() {
	local needle=$1
	if ! grep -Fq -- "$needle" "$LOG"; then
		printf 'FAIL: log does not contain %q\n' "$needle" >&2
		cat "$LOG" >&2
		exit 1
	fi
}

reject_log() {
	local needle=$1
	if grep -Fq -- "$needle" "$LOG"; then
		printf 'FAIL: log unexpectedly contains %q\n' "$needle" >&2
		cat "$LOG" >&2
		exit 1
	fi
}

count_log() {
	local needle=$1
	grep -Fc -- "$needle" "$LOG" || true
}

touch "$TMP/good.mkv" "$TMP/good2.mkv" "$TMP/good space.mkv" "$TMP/refuse.mkv" \
	"$TMP/meta.wav" "$TMP/late-meta.mkv" "$TMP/alpha.png" "$TMP/target.png" \
	"$TMP/reclaim.wav" "$TMP/fail.mkv"

# 1. Batch success: inspect + convert each file, progressbar driven over DBus,
#    one final aggregate popup.
run_wrapper convert --to mp4 "$TMP/good.mkv" "$TMP/good2.mkv" "$TMP/good space.mkv"
expect_status 0
[[ $(count_log 'siphon convert') -eq 3 ]] || {
	printf 'FAIL: expected three convert calls\n' >&2
	cat "$LOG" >&2
	exit 1
}
expect_log 'org.kde.kdialog.ProgressDialog.setValue'
expect_log 'org.kde.kdialog.ProgressDialog.close'
expect_log 'Converted 3 file(s)'
printf 'ok - batch success\n'

# 2. Newer kdialog exposes progress as a D-Bus property rather than setValue.
export QDBUS_PROPERTY_ONLY=1
run_wrapper convert --to mp4 "$TMP/good.mkv"
unset QDBUS_PROPERTY_ONLY
expect_status 0
expect_log 'org.freedesktop.DBus.Properties.Set'
expect_log 'org.kde.kdialog.ProgressDialog value 55'
printf 'ok - kdialog property progress\n'

# 3. Refusal: no convert call, one aggregated error dialog with the code.
run_wrapper convert --to mp4 "$TMP/refuse.mkv"
expect_status 1
reject_log 'siphon convert'
expect_log 'E_CODEC_INCOMPATIBLE'
expect_log ' --error '
printf 'ok - refusal aggregation\n'

# 4. Metadata refusal is prompted, accepted, and retried with --drop-metadata.
run_wrapper convert --to mp3 "$TMP/meta.wav"
expect_status 0
[[ $(count_log 'siphon inspect') -eq 2 ]] || {
	printf 'FAIL: expected metadata retry\n' >&2
	cat "$LOG" >&2
	exit 1
}
expect_log '--drop-metadata'
expect_log 'siphon convert'
printf 'ok - metadata prompt/retry\n'

# 4b. Metadata refusal discovered only during convert is prompted and retried.
run_wrapper convert --to mp4 --mode safe "$TMP/late-meta.mkv"
expect_status 0
[[ $(count_log 'siphon convert') -eq 2 ]] || {
	printf 'FAIL: expected late metadata convert retry\n' >&2
	cat "$LOG" >&2
	exit 1
}
expect_log 'Continue and drop this metadata?'
expect_log '--mode safe'
expect_log '--drop-metadata'
expect_log 'siphon convert'
printf 'ok - late metadata prompt/retry\n'

# 4c. Cancelling the late metadata prompt skips the file without a target.
export KD_WARN_RC=1
run_wrapper convert --to mp4 "$TMP/late-meta.mkv"
unset KD_WARN_RC
expect_status 0
[[ $(count_log 'siphon convert') -eq 1 ]] || {
	printf 'FAIL: cancelled late metadata should not retry\n' >&2
	cat "$LOG" >&2
	exit 1
}
expect_log 'skipped 1'
printf 'ok - late metadata cancel\n'

# 5. Alpha refusal is prompted for a background, then retried.
run_wrapper convert --to jpg "$TMP/alpha.png"
expect_status 0
[[ $(count_log 'siphon inspect') -eq 2 ]] || {
	printf 'FAIL: expected alpha retry\n' >&2
	cat "$LOG" >&2
	exit 1
}
expect_log '--bg white'
expect_log 'Transparency will be flattened'
expect_log 'siphon convert'
printf 'ok - alpha prompt/retry\n'

# 6. Cancelling the reclaim warning skips the file without converting.
export KD_WARN_RC=1
run_wrapper convert --to mp3 --mode reclaim "$TMP/reclaim.wav"
unset KD_WARN_RC
expect_status 0
reject_log 'siphon convert'
expect_log 'skipped 1'
printf 'ok - warning cancel\n'

# 7. One failing file does not stop the rest of the batch.
run_wrapper convert --to mp4 "$TMP/good.mkv" "$TMP/fail.mkv"
expect_status 1
[[ $(count_log 'siphon convert') -eq 2 ]] || {
	printf 'FAIL: batch should attempt both files\n' >&2
	cat "$LOG" >&2
	exit 1
}
expect_log 'E_TEST'
expect_log 'mock conversion failure'
printf 'ok - batch continues after failure\n'

# 8. Without qdbus, the wrapper uses passive popups instead of a progressbar.
: >"$LOG"
set +e
PATH="$MOCK_NO_QDBUS" SIPHON_BIN="$MOCK_NO_QDBUS/siphon" \
	"$WRAPPER" convert --to mp4 "$TMP/good.mkv" >"$TMP/stdout" 2>"$TMP/stderr"
status=$?
set -e
expect_status 0
reject_log '--progressbar'
expect_log '--passivepopup'
expect_log 'Converted 1 file(s)'
printf 'ok - qdbus fallback\n'

# 9. Installer installs both Plasma families, wrapper, and binary; uninstall
#    removes the wrapper and menus without touching unrelated files.
INSTALL="$TMP/install"
"$INSTALLER" --prefix "$INSTALL" --plasma both --no-cache >/dev/null
[[ -x $INSTALL/bin/siphon-gui-kde ]] || {
	printf 'FAIL: wrapper not installed\n' >&2
	exit 1
}
[[ -x $INSTALL/bin/siphon ]] || {
	printf 'FAIL: siphon binary not installed\n' >&2
	exit 1
}
for dir in "$INSTALL/share/kio/servicemenus" "$INSTALL/share/kservices5/ServiceMenus"; do
	[[ $(find "$dir" -maxdepth 1 -name 'siphon-*.desktop' | wc -l) -eq 8 ]] || {
		printf 'FAIL: expected 8 menus in %s\n' "$dir" >&2
		find "$dir" -maxdepth 1 -type f -printf '%f\n' >&2
		exit 1
	}
	grep -Fq "Exec=$INSTALL/bin/siphon-gui-kde convert --to " \
		"$dir/siphon-mkv-to-mp4.desktop" || {
		printf 'FAIL: installed Exec does not use the absolute wrapper path\n' >&2
		grep '^Exec=' "$dir/siphon-mkv-to-mp4.desktop" >&2
		exit 1
	}
	if grep -Fq 'Exec=siphon-gui-kde' "$dir/siphon-mkv-to-mp4.desktop"; then
		printf 'FAIL: installed Exec still relies on PATH\n' >&2
		exit 1
	fi
	[[ -x $dir/siphon-mkv-to-mp4.desktop ]] || {
		printf 'FAIL: installed menu is not executable (KDE will deny it)\n' >&2
		stat -c '%A %n' "$dir/siphon-mkv-to-mp4.desktop" >&2
		exit 1
	}
done
for f in "$ROOT"/kde/servicemenus/*.desktop; do
	grep -Fq 'Type=Service' "$f" || exit 1
	grep -Fq 'MimeType=' "$f" || exit 1
	grep -Fq 'Exec=siphon-gui-kde convert --to ' "$f" || exit 1
	[[ -x $f ]] || {
		printf 'FAIL: source menu is not executable: %s\n' "$f" >&2
		exit 1
	}
done
PATH="$MOCK_NO_QDBUS" "$INSTALL/bin/siphon-gui-kde" --help >/dev/null
"$INSTALLER" --prefix "$INSTALL" --plasma both --no-cache --uninstall >/dev/null
[[ ! -e $INSTALL/bin/siphon-gui-kde ]] || {
	printf 'FAIL: wrapper not removed\n' >&2
	exit 1
}
if find "$INSTALL/share" -name 'siphon-*.desktop' | grep -q .; then
	printf 'FAIL: menus not removed\n' >&2
	exit 1
fi
printf 'ok - install/uninstall\n'

# 10. A prefix containing spaces still yields a valid quoted Exec path.
SPACE_INSTALL="$TMP/install with space"
"$INSTALLER" --prefix "$SPACE_INSTALL" --plasma 6 --no-cache >/dev/null
grep -Fq "Exec=\"$SPACE_INSTALL/bin/siphon-gui-kde\" convert --to " \
	"$SPACE_INSTALL/share/kio/servicemenus/siphon-mkv-to-mp4.desktop" || {
	printf 'FAIL: spaced wrapper path was not quoted\n' >&2
	grep '^Exec=' "$SPACE_INSTALL/share/kio/servicemenus/siphon-mkv-to-mp4.desktop" >&2
	exit 1
}
"$INSTALLER" --prefix "$SPACE_INSTALL" --plasma 6 --no-cache --uninstall >/dev/null
printf 'ok - spaced install path\n'

printf 'all KDE wrapper tests passed\n'
