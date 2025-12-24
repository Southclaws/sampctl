# Runtime configuration reference (`runtime` / `runtimes`)

Runtime configuration lives in your `pawn.json` / `pawn.yaml` under `runtime` (single) or `runtimes` (multiple).

Select a named runtime:

```bash
sampctl run <runtime-name>
```

## Common fields

- `name`: runtime name (used with `sampctl run <name>`).
- `version`: server version to download/run (SA:MP or open.mp).
- `mode`: how the server is run. Common values: `server`, `main`, `y_testing`.
- `runtime_type`: `samp` or `openmp` (usually auto-detected from `version`).

## Server settings

Most `runtime` keys map directly to `server.cfg` settings (and are also used to generate open.mp `config.json` where applicable), for example:

- `port`
- `hostname`
- `maxplayers`
- `rcon_password`
- `lanmode`, `announce`, `query`

## Plugins and components

- `plugins` / `components` are the names to load.
- To download plugins/components, use dependency URL schemes like `plugin://user/repo` and `component://user/repo`.

See: [Runtime configuration guide](configuration.md)

## Extra settings

If you need settings that aren’t covered by dedicated fields, use `extra`:

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
