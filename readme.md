# sampctl

[![Build Status](https://travis-ci.org/Southclaws/sampctl.svg?branch=master)](https://travis-ci.org/Southclaws/sampctl)

A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency.

- auto download of binary packages for either platform
- manage your server settings in JSON format (compiles to server.cfg)
- automatically restarts after crashes and prevents crashloops

## Usage

`sampctl run` Will run a server in your current directory. If there are no binaries, it will automatically download them.

`sampctl download` Just downloads the binaries to the current directory.

`sampctl init` Initialises a server folder by asking some basic questions about the server, mainly for newcomers to SA:MP server hosting.

## An Easier Way To Configure via `samp.json`

Everybody loves JSON! An I've always hated the `server.cfg` structure, so no longer will you need to edit this file by hand! You can work with a modern, structured, JSON format instead.

If your current directory has a JSON file named `samp.json`, the values will be used to generate a `server.cfg` file.

The setting key names are the exact same as [the `server.cfg` settings](http://wiki.sa-mp.com/wiki/Server.cfg) the only difference is how the `gamemodes` settings are handled.

In `server.cfg` for multiple gamemodes, you have multiple `gamemode#` entries, like:

```ini
gamemode0 sumo
gamemode1 golf
gamemode2 tdm
```

But in JSON-land, this looks like:

```json
{
    "gamemodes": [
        "sumo",
        "golf",
        "tdm"
    ]
}
```

Currently, there's no way to set the number of times each gamemode will repeat. If there is demand for this feature, I will implement it.

| key                 | value type       |
|---------------------|------------------|
| `gamemodes`         | array of strings |
| `rcon_password`     | string           |
| `announce`          | bool             |
| `maxplayers`        | int              |
| `port`              | int              |
| `lanmode`           | bool             |
| `query`             | bool             |
| `rcon`              | bool             |
| `logqueries`        | bool             |
| `stream_rate`       | int              |
| `stream_distance`   | float            |
| `sleep`             | string           |
| `maxnpc`            | int              |
| `onfoot_rate`       | int              |
| `incar_rate`        | int              |
| `weapon_rate`       | int              |
| `chatlogging`       | bool             |
| `timestamp`         | bool             |
| `bind`              | string           |
| `password`          | string           |
| `hostname`          | string           |
| `language`          | string           |
| `mapname`           | string           |
| `weburl`            | string           |
| `gamemodetext`      | string           |
| `filterscripts`     | array of strings |
| `plugins`           | array of strings |
| `nosign`            | string           |
| `logtimeformat`     | string           |
| `messageholelimit`  | int              |
| `messageslimit`     | int              |
| `ackslimit`         | int              |
| `playertimeout`     | int              |
| `minconnectiontime` | int              |
| `lagcompmode`       | int              |
| `connseedtime`      | int              |
| `db_logging`        | bool             |
| `db_log_queries`    | bool             |
| `conncookies`       | bool             |
| `cookielogging`     | bool             |

You can also use environment variables to configure, just prefix them with `SAMP_` and uppercase the rest.

For example: `rcon_password`'s environment variable is `SAMP_RCON_PASSWORD`.

## Crashloops and Exponential Backoff

Crashes, crashlooks and backoff timing is handled by the app. If the server crashes, it will be restarted. If it crashes repeatedly, it will be restarted with an exponentially increasing amount of time between tries - in case it's waiting for a database to spin up or something. Once the backoff time reaches 15s, it quits.

## Roadmap

The main focus of this project was to bring SA:MP server management into the present with a modern system `*ctl` tool (think systemd, kubectl, caddy, etc).

Another primary focus was to ease the use of SA:MP in a Docker container, this project is at: https://github.com/Southclaws/docker-samp

The future will include:

- processing of logs to extract meaningful information such as runtime errors
- a RESTful API to control the server that's better than RCON in every possible way
- auto-restart when gamemode .amx files are updated
