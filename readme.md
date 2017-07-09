# sampctl

A small utility for launching a SA:MP server with better settings handling.

May include subcommands for common tasks too.

Proposed usage:

`sampctl --gamemode0=ScavengeSurvive --filterscripts="rcon,objectloader,base" --rcon_password="0xdeadbeef"`

And env vars

`GAMEMODE0=ScavengeSurvive FILTERSCRIPTS=rcon,objectloader,base RCON_PASSWORD=0xdeadbeef sampctl`

Or JSON/Yaml too probably

`sampctl --config=server.json`

As well as some other nice features like log rotation, pre-flight checks for common mistakes and watchdog restarts.
