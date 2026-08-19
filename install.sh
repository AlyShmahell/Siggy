#!/usr/bin/env bash
set -euo pipefail

REPO="AlyShmahell/Siggy"
PREFIX="${PREFIX:-${HOME}/.local}"
BIN="$PREFIX/bin/siggy"
SHARE="$PREFIX/share/siggy"

c_reset=$'\033[0m'
c_bold=$'\033[1m'
c_dim=$'\033[2m'
c_cyan=$'\033[36m'
c_mag=$'\033[35m'
c_yel=$'\033[33m'
c_red=$'\033[31m'
c_inv=$'\033[7m'

die() {
	printf '%serror:%s %s\n' "$c_red" "$c_reset" "$*" >&2
	exit 1
}

need_bash() {
	if [[ -z "${BASH_VERSION:-}" ]]; then
		die "run this installer with bash (curl -fsSL …/install.sh | bash)"
	fi
}

os_arch() {
	local os arch
	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported arch: $arch" ;;
	esac
	case "$os-$arch" in
	linux-amd64 | darwin-arm64) ;;
	*) die "no package for $os/$arch (need linux/amd64 or darwin/arm64)" ;;
	esac
	printf '%s %s\n' "$os" "$arch"
}

checksum() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}

rewrite_desktop() {
	local src="$1"
	local dest="$2"
	local exec_path="$3"
	sed "s|^Exec=.*|Exec=${exec_path} %F|" "$src" >"$dest"
}

warn_deps() {
	local missing=()
	local cmd
	for cmd in bash git rg pdftoppm; do
		if ! command -v "$cmd" >/dev/null 2>&1; then
			missing+=("$cmd")
		fi
	done
	if ((${#missing[@]} > 0)); then
		printf '%smissing tools:%s %s\n' "$c_yel" "$c_reset" "${missing[*]}" >&2
		printf 'install them with your package manager (e.g. git ripgrep poppler).\n' >&2
	fi
}

install_from_dir() {
	local src="$1"
	[[ -x "$src/bin/siggy" ]] || die "no bin/siggy in $src"
	mkdir -p "$PREFIX/bin" "$SHARE/prompts"
	install -m 755 "$src/bin/siggy" "$BIN"
	cp -R "$src/share/prompts/." "$SHARE/prompts/"
	if [[ "$(uname -s)" == "Linux" && -f "$src/share/applications/siggy.desktop" ]]; then
		mkdir -p "$PREFIX/share/applications" "$PREFIX/share/icons/hicolor/scalable/apps"
		rewrite_desktop "$src/share/applications/siggy.desktop" "$PREFIX/share/applications/siggy.desktop" "$BIN"
		if [[ -f "$src/share/icons/hicolor/scalable/apps/siggy.svg" ]]; then
			cp "$src/share/icons/hicolor/scalable/apps/siggy.svg" "$PREFIX/share/icons/hicolor/scalable/apps/siggy.svg"
		fi
		if command -v update-desktop-database >/dev/null 2>&1; then
			update-desktop-database "$PREFIX/share/applications" >/dev/null 2>&1 || true
		fi
	fi
	warn_deps
	case ":$PATH:" in
	*":$PREFIX/bin:"*) ;;
	*) printf 'add %s to PATH, then run: siggy\n' "$PREFIX/bin" ;;
	esac
	printf 'installed %s\n' "$BIN"
}

local_root() {
	local self="${BASH_SOURCE[0]:-$0}"
	if [[ -f "$self" ]]; then
		(cd "$(dirname "$self")" && pwd)
	fi
}

hide() { printf '\033[?25l' >&2; }
show() { printf '\033[?25h' >&2; }
cleanup() {
	show
	stty echo icanon < /dev/tty 2>/dev/null || true
}

draw() {
	local selected="$1"
	shift
	local items=("$@")
	local i
	printf '\033[2J\033[H' >&2
	printf '%s%s siggy %sinstall%s\n\n' "$c_bold" "$c_mag" "$c_reset" "$c_dim" >&2
	for i in "${!items[@]}"; do
		if [[ "$i" -eq "$selected" ]]; then
			printf '  %s%s ▸ %s %s\n' "$c_inv" "$c_cyan" "${items[$i]}" "$c_reset" >&2
		else
			printf '    %s%s%s\n' "$c_dim" "${items[$i]}" "$c_reset" >&2
		fi
	done
	printf '\n%s↑/↓ move   enter select   esc quit%s\n' "$c_yel" "$c_reset" >&2
}

pick_tag() {
	local items=("$@")
	local n="${#items[@]}"
	local idx=0
	local key
	if [[ "$n" -eq 0 ]]; then
		die "no GitHub releases yet"
	fi
	if [[ ! -r /dev/tty ]]; then
		printf '%s\n' "${items[0]}"
		return 0
	fi
	trap cleanup EXIT INT TERM
	stty -echo -icanon < /dev/tty
	hide
	draw "$idx" "${items[@]}"
	while true; do
		if ! IFS= read -rsn1 key < /dev/tty; then
			cleanup
			trap - EXIT INT TERM
			return 1
		fi
		case "$key" in
		$'\033')
			IFS= read -rsn1 -t 0.05 key < /dev/tty || true
			if [[ -z "${key:-}" ]]; then
				cleanup
				trap - EXIT INT TERM
				return 1
			fi
			if [[ "$key" == "[" ]]; then
				IFS= read -rsn1 -t 0.05 key < /dev/tty || true
				case "$key" in
				A) idx=$(((idx - 1 + n) % n)) ;;
				B) idx=$(((idx + 1) % n)) ;;
				esac
			fi
			;;
		"")
			cleanup
			trap - EXIT INT TERM
			printf '%s\n' "${items[$idx]}"
			return 0
			;;
		q | Q)
			cleanup
			trap - EXIT INT TERM
			return 1
			;;
		esac
		draw "$idx" "${items[@]}"
	done
}

list_tags() {
	local json
	json="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=20")" || die "could not list GitHub releases"
	printf '%s' "$json" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
}

download_tag() {
	local tag="$1"
	local os="$2"
	local arch="$3"
	local ver="${tag#v}"
	local name="siggy-${ver}-${os}-${arch}.tar.gz"
	local url="https://github.com/${REPO}/releases/download/${tag}/${name}"
	local tmp
	tmp="$(mktemp -d)"
	curl -fsSL "$url" -o "$tmp/$name" || {
		rm -rf "$tmp"
		die "download failed: $url"
	}
	if curl -fsSL "${url}.sha256" -o "$tmp/$name.sha256" 2>/dev/null; then
		(cd "$tmp" && checksum -c "$name.sha256") || {
			rm -rf "$tmp"
			die "checksum mismatch"
		}
	fi
	tar -xzf "$tmp/$name" -C "$tmp"
	local dir
	dir="$(find "$tmp" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
	if [[ -z "$dir" ]]; then
		rm -rf "$tmp"
		die "tarball had no directory"
	fi
	install_from_dir "$dir"
	rm -rf "$tmp"
}

remote_install() {
	local os arch tag
	read -r os arch < <(os_arch)
	if [[ -n "${SIGGY_VERSION:-}" ]]; then
		tag="${SIGGY_VERSION}"
	else
		tags=()
		while IFS= read -r line; do
			[[ -n "$line" ]] && tags+=("$line")
		done < <(list_tags)
		if [[ ${#tags[@]} -eq 0 ]]; then
			die "no GitHub releases yet; package locally with siggy/build"
		fi
		if [[ -r /dev/tty ]]; then
			tag="$(pick_tag "${tags[@]}")" || die "cancelled"
		else
			tag="${tags[0]}"
		fi
	fi
	download_tag "$tag" "$os" "$arch"
}

need_bash
root="$(local_root || true)"
if [[ -n "$root" && -x "$root/bin/siggy" ]]; then
	install_from_dir "$root"
else
	remote_install
fi
