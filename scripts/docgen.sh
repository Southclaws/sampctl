#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
target="$root/docs/cli-reference.md"
temporary=$(mktemp "$target.XXXXXX")
trap 'rm -f "$temporary"' 0
trap 'exit 1' HUP INT TERM

"$root/sampctl" --bare docs > "$temporary"
mv "$temporary" "$target"
