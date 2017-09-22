# sampctl

A small utility for launching a SA:MP server with better settings handling.

## Usage

`sampctl run` Will run a server in your current directory. If there are no binaries, it will automatically download them.

`sampctl download` Just downloads the binaries to the current directory.

## Config

If your current directory has a JSON file named `samp.json`, the values will be used to generate a `server.cfg` file. The setting names are the same.

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
