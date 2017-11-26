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
