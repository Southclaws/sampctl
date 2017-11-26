# sampctl

[![Build Status](https://travis-ci.org/Southclaws/sampctl.svg?branch=master)](https://travis-ci.org/Southclaws/sampctl)[![Go Report Card](https://goreportcard.com/badge/github.com/Southclaws/sampctl)](https://goreportcard.com/report/github.com/Southclaws/sampctl)

A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency.

- manage your server settings in JSON format (compiles to server.cfg)
- automatically restarts after crashes and prevents crashloops
- auto download of binary packages for either platform

## Installation

- [Linux (Debian/Ubuntu)](https://github.com/Southclaws/sampctl/wiki/Linux)
- [Windows](https://github.com/Southclaws/sampctl/wiki/Windows)
- [Mac](https://github.com/Southclaws/sampctl/wiki/Mac)

## `sampctl`

1.4.0-RC1 - Southclaws <southclaws@gmail.com>

A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency.

## Commands (5)

### `sampctl server`

Usage: `sampctl server <subcommand>`

For managing servers and runtime configurations.

#### Subcommands (3)

### `sampctl server init`

Usage: `sampctl server init`

Bootstrap a new SA:MP server and generates a `samp.json` configuration based on user input. If `gamemodes`, `filterscripts` or `plugins` directories are present, you will be prompted to select relevant files.

#### Flags

- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")

### `sampctl server download`

Usage: `sampctl server download`

Downloads the files necessary to run a SA:MP server to the current directory (unless `--dir` specified). Will download the latest stable (non RC) server version unless `--version` is specified.

#### Flags

- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")

### `sampctl server run`

Usage: `sampctl server run`

Generates a `server.cfg` file based on the configuration inside `samp.json` then executes the server process and automatically restarts it on crashes.

#### Flags

- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")
- `--container`: starts the server as a Linux container instead of running it in the current directory


---

### `sampctl package`

Usage: `sampctl package <subcommand>`

For managing Pawn packages such as gamemodes and libraries.

#### Subcommands (3)

### `sampctl package ensure`

Usage: `sampctl package ensure`

Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`.

#### Flags

- `--dir value`: working directory for the project - by default, uses the current directory (default: ".")

### `sampctl package build`

Usage: `sampctl package build`

Builds a package defined by a `pawn.json` or `pawn.yaml` file.

#### Flags

- `--dir value`: working directory for the project - by default, uses the current directory (default: ".")
- `--build --forceBuild`: build configuration to use if --forceBuild is set
- `--forceEnsure --forceBuild`: forces dependency ensure before build if --forceBuild is set

### `sampctl package run`

Usage: `sampctl package run`

Compiles and runs a package defined by a `pawn.json` or `pawn.yaml` file.

#### Flags

- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")
- `--container`: starts the server as a Linux container instead of running it in the current directory
- `--build --forceBuild`: build configuration to use if --forceBuild is set
- `--forceBuild`: forces a build to run before executing the server
- `--forceEnsure --forceBuild`: forces dependency ensure before build if --forceBuild is set


---

### `sampctl version`

Show version number - this is also the version of the container image that will be used for `--container` runtimes.

---

### `sampctl docs`

Usage: `sampctl docs > documentation.md`

Generate documentation in markdown format and print to standard out.

---

### `sampctl help`

Usage: `Shows a list of commands or help for one command`

---

## Global Flags

- `--help, -h`: show help
- `--appVersion, -V`: sampctl version

## An Easier Way To Configure via `samp.json`

Everybody loves JSON! I've always hated the `server.cfg` structure, so no longer will you need to edit this file by hand! You can work with a modern, structured, JSON format instead.

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