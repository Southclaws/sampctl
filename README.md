# sampctl

[![Build Status](https://travis-ci.org/Southclaws/sampctl.svg?branch=master)](https://travis-ci.org/Southclaws/sampctl)

A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency.

- manage your server settings in JSON format (compiles to server.cfg)
- automatically restarts after crashes and prevents crashloops
- auto download of binary packages for either platform

## Installation

- [Linux (Debian/Ubuntu)](https://github.com/Southclaws/sampctl/wiki/Linux)
- [Windows](https://github.com/Southclaws/sampctl/wiki/Windows)
- [Mac](https://github.com/Southclaws/sampctl/wiki/Mac)

# `sampctl`
1.2.6-R3 - Southclaws <southclaws@gmail.com>

A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency.

## Subcommands

### `init`

initialise a sa-mp server folder with a few questions, uses the cwd if --dir is not set

#### Flags

- `--version`
- `--dir`
- `--endpoint`


### `run`

run a sa-mp server, uses the cwd if --dir is not set

#### Flags

- `--version`
- `--dir`
- `--endpoint`
- `--container`


### `download`

download a version of the server, uses latest if --version is not specified

#### Flags

- `--version`
- `--dir`
- `--endpoint`


### `exec`

execute an amx file as a SA:MP gamemode for quick testing in a temporary server installation

#### Flags

- `--version`
- `--endpoint`
- `--container`


### `compile`

compile a .pwn file to an .amx file in the same directory

#### Flags

- `--pawncc-version`


### `docgen`

generate documentation - mainly just for CI usage, the readme file will always be up to date.

### `help`

Shows a list of commands or help for one command

## Global Flags

- `--help, h`
- `--app-version, V`

## An Easier Way To Configure via `samp.json`

Everybody loves JSON! An I've always hated the `server.cfg` structure, so no longer will you need to edit this file by hand! You can work with a modern, structured, JSON format instead.

If your current directory has a JSON file named `samp.json`, the values will be used to generate a `server.cfg` file.

```json
{
	"gamemodes": [
		"rivershell"
	],
	"plugins": [
		"filemanager"
	],
	"rcon_password": "test",
	"port": 8080,
}
```

Becomes (On Linux - the `.so` extension is automatically added for Linux and omitted on Windows)

```conf
gamemode0 rivershell
plugins filemanager.so
rcon_password test
port 8080
(... and the rest of the settings which have default values)
```

[See documentation for more info.](https://github.com/Southclaws/sampctl/wiki/samp.json-Reference)

## Crashloops and Exponential Backoff

Crashes, crashloops and backoff timing is handled by the app. If the server crashes, it will be restarted. If it crashes repeatedly, it will be restarted with an exponentially increasing amount of time between tries - in case it's waiting for a database to spin up or something. Once the backoff time reaches 15s, it quits.

## Development

Grab the code:

```bash
go get github.com/Southclaws/sampctl
```

Grab the dependencies:

```bash
dep ensure -update
```

Hack away!

## Roadmap

The main focus of this project was to bring SA:MP server management into the present with a modern system `*ctl` tool (think systemd, kubectl, caddy, etc).

Another primary focus was to ease the use of SA:MP in a Docker container, via the `--container` flag this is now extremely easy!

The future will include:

- automatic log management and rotation
- sending error/warning alerts to services such as Discord and IRC
- a RESTful API to control the server that's better than RCON in every possible way
- auto-restart when gamemode .amx files are updated - JavaScript style!