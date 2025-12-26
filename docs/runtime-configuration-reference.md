# Runtime configuration reference (`runtime` / `runtimes`)

Runtime configuration lives in your `pawn.json` / `pawn.yaml` under `runtime` (single) or `runtimes` (multiple).

Select a named runtime:

```bash
sampctl run <runtime-name>
```

## SA:MP vs open.mp runtimes

`sampctl` supports two runtime types:

- **SA:MP**: generates `server.cfg`.
- **open.mp**: generates `config.json` (and may also generate a minimal `server.cfg` for legacy plugin compatibility).

Runtime type is determined by:

- `runtime.runtime_type` (explicit override), or
- auto-detection from `runtime.version` (if it contains `openmp` or `open.mp`, it is treated as open.mp).

## Common fields

- `name`: runtime name (used with `sampctl run <name>`).
- `version`: runtime version string.
- `runtime_type`: `samp` or `openmp` (auto-detected from `version` if not set).
- `mode`: run mode: `server`, `main`, `y_testing`.
- `rootLink`: (sampctl internal) whether to create a symlink to the package root in the runtime directory.
- `echo`: (sampctl internal) an optional string written to the start of the generated config.

## Scripts and load lists

- `gamemodes` (string[]): main scripts to run.
- `filterscripts` (string[]): side scripts to run.
- `plugins` (string[]): plugin names to load.
- `components` (string[]): open.mp components to load.

Notes:

- For open.mp, `plugins` are written to `config.json` as `pawn.legacy_plugins` (extensions stripped).
- For open.mp, `components` are written to `config.json` as `pawn.components` (extensions stripped).

## Server settings

All of the following keys are accepted in `runtime`.

- For **SA:MP**, these map directly to `server.cfg` keys.
- For **open.mp**, a subset is mapped into `config.json` (documented below); open.mp-only fields are also available directly under `runtime`.

### Core

- `rcon_password` (string)
- `port` (int)
- `hostname` (string)
- `maxplayers` (int)
- `language` (string)
- `mapname` (string)
- `weburl` (string)
- `gamemodetext` (string)

### Network and technical

- `bind` (string)
- `password` (string)
- `announce` (bool)
- `lanmode` (bool)
- `query` (bool)
- `rcon` (bool)
- `logqueries` (bool)
- `sleep` (int)
- `maxnpc` (int)

### Rates and performance

- `stream_rate` (int)
- `stream_distance` (number)
- `onfoot_rate` (int)
- `incar_rate` (int)
- `weapon_rate` (int)
- `chatlogging` (bool)
- `timestamp` (bool)
- `nosign` (string)
- `logtimeformat` (string)
- `messageholelimit` (int)
- `messageslimit` (int)
- `ackslimit` (int)
- `playertimeout` (int)
- `minconnectiontime` (int)
- `lagcompmode` (int)
- `connseedtime` (int)
- `db_logging` (bool)
- `db_log_queries` (bool)
- `conncookies` (bool)
- `cookielogging` (bool)
- `output` (bool)

## open.mp `config.json` mapping (what is actually written)

When the runtime is open.mp, `sampctl` generates `config.json` and maps these `runtime` fields:

- `hostname` → `name`
- `maxplayers` → `max_players`
- `language` → `language`
- `password` → `password`
- `announce` → `announce`
- `query` → `enable_query`
- `weburl` → `website`
- `sleep` → `sleep`

Nested `game`:

- `lagcompmode` → `game.lag_compensation_mode`
- `mapname` → `game.map`
- `gamemodetext` → `game.mode`

Nested `network`:

- `port` → `network.port`
- `bind` → `network.bind`
- `onfoot_rate` → `network.on_foot_sync_rate`
- `incar_rate` → `network.in_vehicle_sync_rate`
- `weapon_rate` → `network.aiming_sync_rate`
- `stream_rate` → `network.stream_rate`
- `stream_distance` → `network.stream_radius`
- `messageholelimit` → `network.message_hole_limit`
- `messageslimit` → `network.messages_limit`
- `ackslimit` → `network.acks_limit`
- `playertimeout` → `network.player_timeout`
- `minconnectiontime` → `network.minimum_connection_time`
- `connseedtime` → `network.cookie_reseed_time`
- `lanmode` → `network.use_lan_mode`

Nested `logging`:

- `output` → `logging.enable`
- `chatlogging` → `logging.log_chat`
- `logqueries` → `logging.log_queries`
- `cookielogging` → `logging.log_cookies`
- `db_logging` → `logging.log_sqlite`
- `db_log_queries` → `logging.log_sqlite_queries`
- `timestamp` → `logging.use_timestamp`
- `logtimeformat` → `logging.timestamp_format`

Nested `rcon`:

- `rcon` → `rcon.enable`
- `rcon_password` → `rcon.password`

Nested `pawn`:

- `plugins` → `pawn.legacy_plugins`
- `components` → `pawn.components`
- `gamemodes` → `pawn.main_scripts`
- `filterscripts` → `pawn.side_scripts`

## Plugins and components

- To download plugins/components automatically, prefer dependency URL schemes like `plugin://user/repo` and `component://user/repo`.

See: [Runtime configuration guide](configuration.md)

## Extra settings

If you need extra SA:MP `server.cfg` keys (or simple top-level open.mp keys), use `extra`:

```json
{
  "runtime": {
    "port": 7777,
    "hostname": "My Server",
    "extra": {
      "my_plugin_setting": "1",
      "another_setting": "foo"
    }
  }
}
```

For SA:MP, `extra` is written to `server.cfg`.

For open.mp, `extra` is written to `config.json` (and `sampctl` may also generate a minimal `server.cfg` for legacy plugin compatibility).

## open.mp-only fields

open.mp supports additional `config.json` keys that SA:MP does not. These can be set directly under `runtime` and are only used when the runtime is open.mp.

Supported open.mp-only keys:

- `max_bots` (int)
- `use_dyn_ticks` (bool)
- `logo` (string)
- `game` (object)
- `network` (object)
- `logging` (object)
- `pawn` (object)
- `discord` (object)
- `banners` (object)
- `artwork` (object)

Note: `rcon` is a boolean in SA:MP runtime config; to set open.mp-only `config.json.rcon` fields, use `rcon_config` (object), which is merged into `config.json.rcon`.

```yaml
runtime:
  version: 1.2.0-openmp
  hostname: My Server
  max_bots: 100
  use_dyn_ticks: false
  discord:
    invite: https://discord.gg/example
  network:
    public_addr: 127.0.0.1
  rcon_config:
    allow_teleport: true
```
