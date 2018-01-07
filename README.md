# sampctl

[![Build Status](https://travis-ci.org/Southclaws/sampctl.svg?branch=master)](https://travis-ci.org/Southclaws/sampctl) [![Go Report Card](https://goreportcard.com/badge/github.com/Southclaws/sampctl)](https://goreportcard.com/report/github.com/Southclaws/sampctl) [![https://img.shields.io/badge/Ko--Fi-Buy%20Me%20a%20Coffee-brown.svg](https://img.shields.io/badge/Ko--Fi-Buy%20Me%20a%20Coffee-brown.svg)](https://ko-fi.com/southclaws)

![sampctl-logo](sampctl-wordmark.png)

The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer.

## Features

### Package Manager

Always have the libraries you need. Inspired by npm.

![images/sampctl-package-ensure.gif](images/sampctl-package-ensure.gif)

### Build/Run Tool

Use on the command-line or integrate with any editor.

![images/sampctl-package-build-vscode.gif](images/sampctl-package-build-vscode.gif)

Easily write and run tests for libraries or quickly run arbitrary code. Utilise the power of Docker to run on any platform!

![images/sampctl-package-run-container.gif](images/sampctl-package-run-container.gif)

### Developer Tools

Quickly bootstrap new packages.

![images/sampctl-package-init.gif](images/sampctl-package-init.gif)

### SA:MP Server Configuration - no more `server.cfg`

Manage your server settings in JSON or YAML format

![images/sampctl-server-init.gif](images/sampctl-server-init.gif)

### Automatic Server Restart - no more dodgy bash scripts

Run the server from `sampctl` and let it worry about restarting in case of crashes.

![images/sampctl-server-run.gif](images/sampctl-server-run.gif)

### Automatic Server and Plugin Installer

Automatically download Windows/Linux server binaries and plugins when and where you need them.

![images/sampctl-server-ensure.gif](images/sampctl-server-ensure.gif)

## Installation

Installation is simple and fast on all platforms so why not give sampctl a try?

* [Linux (Debian/Ubuntu)](https://github.com/Southclaws/sampctl/wiki/Linux)
* [Windows](https://github.com/Southclaws/sampctl/wiki/Windows)
* [Mac](https://github.com/Southclaws/sampctl/wiki/Mac)

## Usage

Scroll to the end of this document for an overview of the commands.

Or visit the [wiki](https://github.com/Southclaws/sampctl/wiki) for all the information you need.

---

## Overview

sampctl is designed for both development of gamemodes/libraries and management of live servers.

Below is a quick overview of the best features that will help _you_ develop faster.

### Package Management and Build Tool

If you've used platforms like NodeJS, Python, Go, Ruby, etc you know how useful tools like npm, pip, gem are.

It's about time Pawn had the same tool.

sampctl provides a simple and intuitive way to _declare_ what includes your project needs. After that you simply let sampctl take care of the downloading and building.

If you release scripts, you know it's awkward to test even simple code. You need to set up a server, compile the include into a gamemode, configure the server and run it.

Forget all that. Just make a `pawn.json` in your project directory with `sampctl package init` and use `sampctl package install` the includes you need:

```json
{
    "entry": "test.pwn",
    "output": "test.amx",
    "dependencies": ["Southclaws/samp-stdlib", "Southclaws/formatex"]
}
```

Write your quick test code:

```pawn
#include <a_samp>
#include <formatex>

main() {
    new str[128];
    formatex(str, sizeof str, "My favourite vehicle is: '%%v'!", 400); // should print "Landstalker"
    print(str);
}
```

And run it!

```bash
sampctl package run

Server Plugins
--------------
 Loaded 0 plugins.

Started server on port: 7777, with maxplayers: 50 lanmode is OFF.

Filterscripts
---------------
  Loaded 0 filterscripts.

My favourite vehicle is: 'Landstalker'!
```

You get the compiler output and the server output without ever needing to:

* visit sa-mp.com/download.php
* unzip a server package
* worry about Windows or Linux differences
* set up the Pawn compiler with your favourite editor
* make sure the Pawn compiler is reading the correct includes
* download the formatex include

[See documentation for more info.](https://github.com/Southclaws/sampctl/wiki/Packages)

### Server Configuration and Automatic Plugin Download

Use JSON or YAML to write your server config:

```json
{
    "gamemodes": ["rivershell"],
    "plugins": ["maddinat0r/sscanf"],
    "rcon_password": "test",
    "port": 8080
}
```

It compiles to this:

```conf
gamemode0 rivershell
plugins sscanf.so
rcon_password test
port 8080
(... and the rest of the settings which have default values)
```

What also happens here is `maddinat0r/sscanf` tells sampctl to automatically get the latest sscanf plugin and place the `.so` or `.dll` file into the `plugins/` directory.

[See documentation for more info.](https://github.com/Southclaws/sampctl/wiki/Runtime-Configuration-Reference)

---
# `sampctl`

1.5.14 - Southclaws <southclaws@gmail.com>

Compiles server configuration JSON to server.cfg format. Executes the server and monitors it for crashes, restarting if necessary. Provides a way to quickly download server binaries of a specified version. Provides dependency management and package build tools for library maintainers and gamemode writers alike.

## Commands (5)

### `sampctl server`

Usage: `sampctl server <subcommand>`

For managing servers and runtime configurations.

#### Subcommands (4)

### `sampctl server init`

Usage: `sampctl server init`

Bootstrap a new SA:MP server and generates a `samp.json`/`samp.yaml` configuration based on user input. If `gamemodes`, `filterscripts` or `plugins` directories are present, you will be prompted to select relevant files.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")

### `sampctl server download`

Usage: `sampctl server download`

Downloads the files necessary to run a SA:MP server to the current directory (unless `--dir` specified). Will download the latest stable (non RC) server version unless `--version` is specified.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")

### `sampctl server ensure`

Usage: `sampctl server ensure`

Ensures the server environment is representative of the configuration specified in `samp.json`/`samp.yaml` - downloads server binaries and plugin files if necessary and generates a `server.cfg` file.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--noCache --forceEnsure`: forces download of plugins if --forceEnsure is set

### `sampctl server run`

Usage: `sampctl server run`

Generates a `server.cfg` file based on the configuration inside `samp.json`/`samp.yaml` then executes the server process and automatically restarts it on crashes.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--container`: starts the server as a Linux container instead of running it in the current directory
- `--mountCache --container`: if --container is set, mounts the local cache directory inside the container
- `--forceEnsure`: forces plugin and binaries ensure before run
- `--noCache --forceEnsure`: forces download of plugins if --forceEnsure is set


---

### `sampctl package`

Usage: `sampctl package <subcommand>`

For managing Pawn packages such as gamemodes and libraries.

#### Subcommands (5)

### `sampctl package init`

Usage: `sampctl package init`

Helper tool to bootstrap a new package or turn an existing project into a package.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--dir value`: working directory for the project - by default, uses the current directory (default: ".")

### `sampctl package ensure`

Usage: `sampctl package ensure`

Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`/`pawn.yaml`.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--dir value`: working directory for the project - by default, uses the current directory (default: ".")

### `sampctl package install`

Usage: `sampctl package install [package definition]`

Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` downloads the contents.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--dir value`: working directory for the project - by default, uses the current directory (default: ".")
- `--dev`: for specifying dependencies only necessary for development or testing of the package

### `sampctl package build`

Usage: `sampctl package build`

Builds a package defined by a `pawn.json`/`pawn.yaml` file.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--dir value`: working directory for the project - by default, uses the current directory (default: ".")
- `--build --forceBuild`: build configuration to use if --forceBuild is set
- `--forceEnsure --forceBuild`: forces dependency ensure before build if --forceBuild is set

### `sampctl package run`

Usage: `sampctl package run`

Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.

#### Flags

- `--verbose`: output all detailed information - useful for debugging
- `--version value`: the SA:MP server version to use (default: "0.3.7")
- `--dir value`: working directory for the server - by default, uses the current directory (default: ".")
- `--endpoint value`: endpoint to download packages from (default: "http://files.sa-mp.com")
- `--container`: starts the server as a Linux container instead of running it in the current directory
- `--mountCache --container`: if --container is set, mounts the local cache directory inside the container
- `--build --forceBuild`: build configuration to use if --forceBuild is set
- `--forceBuild`: forces a build to run before executing the server
- `--forceEnsure --forceBuild`: forces dependency ensure before build if --forceBuild is set
- `--noCache --forceEnsure`: forces download of plugins if --forceEnsure is set


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

- `--verbose`: output all detailed information - useful for debugging
- `--help, -h`: show help
- `--appVersion, -V`: sampctl version


