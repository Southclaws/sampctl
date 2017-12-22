# sampctl

[![Build Status](https://travis-ci.org/Southclaws/sampctl.svg?branch=master)](https://travis-ci.org/Southclaws/sampctl) [![Go Report Card](https://goreportcard.com/badge/github.com/Southclaws/sampctl)](https://goreportcard.com/report/github.com/Southclaws/sampctl) [![https://img.shields.io/badge/Ko--Fi-Buy%20Me%20a%20Coffee-brown.svg](https://img.shields.io/badge/Ko--Fi-Buy%20Me%20a%20Coffee-brown.svg)](https://ko-fi.com/southclaws)

![sampctl.png](sampctl.png)

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

Installation is simple and fast on all platforms. If you're not into it, uninstallation is also simple and fast.

* [Linux (Debian/Ubuntu)](https://github.com/Southclaws/sampctl/wiki/Linux)
* [Windows](https://github.com/Southclaws/sampctl/wiki/Windows)
* [Mac](https://github.com/Southclaws/sampctl/wiki/Mac)

## Usage

Scroll to the end of this document for an overview of the commands.

Or visit the [wiki](https://github.com/Southclaws/sampctl/wiki) for all the information you need.

---

## Features

sampctl is designed for both development of gamemodes/libraries and management of live servers.

### Package Management and Build Tool

If you've used platforms like NodeJS, Python, Go, Ruby, etc you know how useful tools like npm, pip, gem are.

It's about time Pawn had the same tool.

sampctl provides a simple and intuitive way to _declare_ what includes your project depends on while taking care of all the hard work such as downloading those includes to the correct directory, ensuring they are at the correct version and making sure the compiler has all the information it needs.

If you're a Pawn library maintainer, you know it's awkward to set up unit tests for libraries. Even if you just want to quickly test some code, you know that you can't just write code and test it instantly. You need to set up a server, compile the include into a gamemode, configure the server and run it.

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
    formatex(str, sizeof str, "My favourite vehicle is: '%%v'!", 400); // should print "Landstalker"
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

[See documentation for more info.](https://github.com/Southclaws/sampctl/wiki/Package-Definition-Reference)

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
plugins filemanager.so
rcon_password test
port 8080
(... and the rest of the settings which have default values)
```

What also happens here is `maddinat0r/sscanf` tells sampctl to automatically get the latest sscanf plugin and place the `.so` or `.dll` file into the `plugins/` directory.

[See documentation for more info.](https://github.com/Southclaws/sampctl/wiki/Runtime-Configuration-Reference)

---
