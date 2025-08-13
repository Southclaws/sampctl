#!/usr/bin/env bash
# a simple install script for sampctl

ARCH=$(uname -m)
PATTERN="browser_download_url.*386\.deb"

if [ $ARCH = "x86_64" ]; then
	PATTERN="browser_download_url.*amd64\.deb"
fi

curl -s https://api.github.com/repos/Southclaws/sampctl/releases/latest |
	grep $PATTERN |
	cut -d : -f 2,3 |
	tr -d \" |
	wget -qi - -O tmp.deb
sudo dpkg -i tmp.deb
rm tmp.deb
