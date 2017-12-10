#!/usr/bin/env bash
# a simple download script for sampctl

ARCH=$(uname -p)
PATTERN="browser_download_url.*linux_386\.tar"

if [ $ARCH = "x86_64" ]; then
	PATTERN="browser_download_url.*linux_amd64\.tar"
fi

URL=$(curl -s https://api.github.com/repos/Southclaws/sampctl/releases/latest |
	grep $PATTERN |
	cut -d : -f 2,3 |
	tr -d \")

curl -Ls $URL -o sampctl
