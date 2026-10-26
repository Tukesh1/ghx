#!/bin/sh
# ghx installer — downloads a prebuilt release binary; no clone or Go toolchain needed.
#
#   curl -fsSL https://raw.githubusercontent.com/Tukesh1/ghx/main/install.sh | sh
#
# Options (environment variables):
#   GHX_VERSION=v0.1.0       install a specific release instead of the latest
#   GHX_INSTALL_DIR=~/bin    install somewhere other than /usr/local/bin or ~/.local/bin
#   GHX_BASE_URL=...         download from a mirror (used by tests)
#
# Uninstall: rm "$(command -v ghx)"

set -eu

REPO="Tukesh1/ghx"
VERSION="${GHX_VERSION:-latest}"

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
fail() {
	printf '\033[1;31merror:\033[0m %s\n' "$*" >&2
	exit 1
}

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 3 -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "curl or wget is required"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo ""
	fi
}

detect_platform() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	case "$os" in
	darwin | linux) ;;
	mingw* | msys* | cygwin*) fail "on Windows use: irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex" ;;
	*) fail "unsupported operating system: $os" ;;
	esac

	arch=$(uname -m)
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported architecture: $arch" ;;
	esac

	# Rosetta reports x86_64 on Apple Silicon; prefer the native binary.
	if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
		arch=arm64
	fi
}

pick_install_dir() {
	if [ -n "${GHX_INSTALL_DIR:-}" ]; then
		dir="$GHX_INSTALL_DIR"
	elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		dir=/usr/local/bin
	else
		dir="$HOME/.local/bin"
	fi
	mkdir -p "$dir" || fail "cannot create $dir"
	[ -w "$dir" ] || fail "$dir is not writable (set GHX_INSTALL_DIR)"
}

main() {
	detect_platform
	asset="ghx_${os}_${arch}.tar.gz"
	if [ -n "${GHX_BASE_URL:-}" ]; then
		base="$GHX_BASE_URL"
	elif [ "$VERSION" = latest ]; then
		base="https://github.com/$REPO/releases/latest/download"
	else
		base="https://github.com/$REPO/releases/download/$VERSION"
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	info "Downloading ghx ($VERSION) for $os/$arch"
	download "$base/$asset" "$tmp/$asset" || fail "download failed: $base/$asset"

	if download "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
		want=$(awk -v f="$asset" '$2 == f {print $1}' "$tmp/checksums.txt")
		got=$(sha256 "$tmp/$asset")
		if [ -z "$want" ] || [ -z "$got" ]; then
			warn "could not verify checksum"
		elif [ "$want" != "$got" ]; then
			fail "checksum mismatch for $asset (expected $want, got $got)"
		else
			info "Checksum verified"
		fi
	else
		warn "checksums.txt not found; skipping verification"
	fi

	tar -xzf "$tmp/$asset" -C "$tmp" ghx || fail "could not extract $asset"
	pick_install_dir
	mv "$tmp/ghx" "$dir/ghx"
	chmod 755 "$dir/ghx"
	info "Installed $("$dir/ghx" --version 2>/dev/null || echo ghx) to $dir/ghx"

	case ":$PATH:" in
	*":$dir:"*) ;;
	*)
		warn "$dir is not on your PATH. Add this to your shell profile:"
		echo
		echo "    export PATH=\"$dir:\$PATH\""
		echo
		;;
	esac

	if ! command -v gh >/dev/null 2>&1; then
		warn "GitHub CLI (gh) not found. For live data install it from https://cli.github.com and run: gh auth login"
		warn "Or try the offline demo now: ghx --demo"
	fi
	info "Run: ghx   (or ghx --demo)"
	info "Tip: GitHub Octicons need a Nerd Font in your terminal — https://www.nerdfonts.com"
}

main "$@"
