# sampctl

[![Build Status](https://travis-ci.org/Southclaws/sampctl.svg?branch=master)](https://travis-ci.org/Southclaws/sampctl)[![Go Report Card](https://goreportcard.com/badge/github.com/Southclaws/sampctl)](https://goreportcard.com/report/github.com/Southclaws/sampctl)

The Swiss Army Knife of SA:MP - vital tools for any server owner or library
maintainer.

## Overview

Server management and configuration tools:

* Manage your server settings in JSON format (compiles to server.cfg)
* Run the server from `sampctl` and let it worry about automatic restarts
* Automatically download Windows/Linux server binaries when you need them

Package management and dependency tools:

* Always have the libraries you need at the versions to specify
* No more copies of the Pawn compiler or includes, let `sampctl` handle it
* Easily write and run tests for libraries or quickly run arbitrary code

## Installation

* [Linux (Debian/Ubuntu)](https://github.com/Southclaws/sampctl/wiki/Linux)
* [Windows](https://github.com/Southclaws/sampctl/wiki/Windows)
* [Mac](https://github.com/Southclaws/sampctl/wiki/Mac)

## `sampctl`

1.4.0-RC3 - Southclaws <southclaws@gmail.com>

Compiles server configuration JSON to server.cfg format. Executes the server and monitors it for crashes, restarting if necessary. Provides a way to quickly download server binaries of a specified version. Provides dependency management and package build tools for library maintainers and gamemode writers alike.

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

Everybody loves JSON! I've always hated the `server.cfg` structure, so no longer
will you need to edit this file by hand! You can work with a modern, structured,
JSON format instead.

If your `samp.json` looks like this:

```json
{
    "gamemodes": ["rivershell"],
    "plugins": ["filemanager"],
    "rcon_password": "test",
    "port": 8080
}
```

It compiles to this:

```conf
gamemode0 rivershell
plugins filemanager.so
rcon_password test
port 8080
(... and the rest of the settings which have default values)
```

Note that the plugins line turned `filemanager` into `filemanager.so` because
this example was run on a Linux machine.

[See documentation for more info.](https://github.com/Southclaws/sampctl/wiki/samp.json-Reference)

## Write libraries like it's npm with `pawn.json`

Not writing a gamemode? If you're a Pawn library maintainer, you know it's
awkward to set up unit tests for libraries. Even if you just want to quickly
test some code, you have to provision a server, set the gamemode in the
server.cfg, write and compile code using the correct compiler.

Forget all that. Just make a `pawn.json` in your project directory:

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
    formatex(str, sizeof str, "My favourite vehicle is: '%v'!", 400); // should print "Landstalker"
    print(str);
}
```

And run it!

```bash
sampctl package run
Using cached package for 0.3.7
building /: with 3.10.4
Compiling source: '/tmp/test.pwn' with compiler 3.10.4...
Using cached package pawnc-3.10.4-darwin.zip
Pawn compiler 3.10.2                    Copyright (c) 1997-2006, ITB CompuPhase

Header size:            480 bytes
Code size:             5960 bytes
Data size:            15876 bytes
Stack/heap size:      16384 bytes; estimated max. usage=300 cells (1200 bytes)
Total requirements:   38700 bytes
Starting server...

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
* worry about Windows or Linux
* set up the Pawn compiler
* make sure the Pawn compiler is reading the correct includes
* download the formatex include

## Crashloops and Exponential Backoff

Crashes, crashloops and backoff timing is handled by the app. If the server
crashes, it will be restarted. If it crashes repeatedly, it will be restarted
with an exponentially increasing amount of time between tries - in case it's
waiting for a database to spin up or something. Once the backoff time reaches
15s, it quits.

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