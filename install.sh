#!/bin/sh
set -eu

repo="thomasjdelaney/TwitchLiveCLI"
install_dir=${INSTALL_DIR:-"${HOME:-}/.local/bin"}

if [ -z "${HOME:-}" ] && [ -z "${INSTALL_DIR:-}" ]; then
	printf '%s\n' 'Set HOME or INSTALL_DIR before running the installer.' >&2
	exit 1
fi

case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) printf 'Unsupported operating system: %s\n' "$(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) printf 'Unsupported architecture: %s\n' "$(uname -m)" >&2; exit 1 ;;
esac

asset="twitch-live-cli_${os}_${arch}"
url="https://github.com/${repo}/releases/latest/download/${asset}.tar.gz"
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/twitch-live-cli-install.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

if command -v curl >/dev/null 2>&1; then
	curl -fsSL "$url" -o "$tmp_dir/archive.tar.gz"
elif command -v wget >/dev/null 2>&1; then
	wget -q "$url" -O "$tmp_dir/archive.tar.gz"
else
	printf '%s\n' 'Install curl or wget, then run this installer again.' >&2
	exit 1
fi

tar -xzf "$tmp_dir/archive.tar.gz" -C "$tmp_dir" "$asset"
mkdir -p "$install_dir"
cp "$tmp_dir/$asset" "$install_dir/twitch-live-cli"
chmod 755 "$install_dir/twitch-live-cli"
printf 'Installed twitch-live-cli to %s/twitch-live-cli\n' "$install_dir"
case ":${PATH}:" in
	*":${install_dir}:"*) ;;
	*) printf 'Add %s to your PATH to run twitch-live-cli.\n' "$install_dir" ;;
esac
