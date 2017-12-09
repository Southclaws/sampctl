#!/usr/bin/env bash
# a simple install script for sampctl

ARCH=$(uname -p)
PATTERN="browser_download_url.*386\.rpm"

if [ $ARCH = "x86_64" ]; then
	PATTERN="browser_download_url.*amd64\.rpm"
fi

URL=$(curl -s https://api.github.com/repos/Southclaws/sampctl/releases/latest |
	grep $PATTERN |
	cut -d : -f 2,3 |
	tr -d \")

curl -Ls $URL -o tmp.rpm
rpm -Uvh tmp.rpm
rm tmp.rpm
