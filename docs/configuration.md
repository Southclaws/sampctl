# Runtime configuration (in `pawn.json` / `pawn.yaml`)

You don’t edit `server.cfg` by hand.

Instead, `sampctl` generates:

- `server.cfg` for SA:MP
- `config.json` for open.mp

…based on the `runtime`/`runtimes` section in your `pawn.json` / `pawn.yaml`.

## Example

`pawn.json`:

```json
{
  "entry": "test.pwn",
  "output": "gamemodes/test.amx",
  "preset": "samp",
  "runtime": {
    "rcon_password": "change-me",
    "port": 7777,
    "hostname": "My Server",
    "maxplayers": 50
  }
}
```

Note: when you use `sampctl run`, it automatically runs your built output as the gamemode (you don’t need to set `gamemodes`).

## Plugins and components

- `runtime.plugins` / `runtime.components` are *just the names to load* from `./plugins` or `./components`.
- To have `sampctl` *download* plugins/components for you, add them as dependencies using URL schemes:
  - `plugin://user/repo`
  - `component://user/repo` (open.mp)

After changing dependencies:

```bash
sampctl ensure
```

![Ensure runtime files](images/sampctl-server-ensure.gif)

## Multiple runtime configs

You can define multiple entries under `runtimes` (each one should have a `name`) and select one at run time:

```bash
sampctl run <runtime-name>
```

## Files created by sampctl

Depending on what’s missing, `sampctl` may create:

- `./dependencies/` (downloaded packages)
- `./plugins/` (plugin binaries)
- `./components/` (open.mp components)
- `server.cfg` / `config.json` (generated)
- `.sampctl/` (runtime install manifest)

See also:

- [Global configuration](global-config.md)
- [Dependencies and version pinning](dependencies.md)
- [Runtime configuration reference](runtime-configuration-reference.md)
