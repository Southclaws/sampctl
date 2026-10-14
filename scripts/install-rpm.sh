#!/bin/sh
set -eu

case "$(uname -m)" in
	x86_64 | amd64) architecture=amd64 ;;
	aarch64 | arm64) architecture=arm64 ;;
	i386 | i486 | i586 | i686) architecture=386 ;;
	armv7 | armv7l) architecture=armv7 ;;
	*)
		printf 'unsupported architecture: %s\n' "$(uname -m)" >&2
		exit 1
		;;
esac

release=$(curl -fsSL https://api.github.com/repos/Southclaws/sampctl/releases/latest)
url=$(printf '%s\n' "$release" | awk -F '"' -v ending="_$architecture.rpm" '
	$2 == "browser_download_url" && substr($4, length($4) - length(ending) + 1) == ending {
		print $4
		exit
	}')
if [ -z "$url" ]; then
	printf 'no matching linux release asset for %s\n' "$architecture" >&2
	exit 1
fi

temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' 0
trap 'exit 1' HUP INT TERM

curl -fsSL "$url" -o "$temporary/sampctl.rpm"
rpm -Uvh "$temporary/sampctl.rpm"
