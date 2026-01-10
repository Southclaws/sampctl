# Migrating from SA:MP to open.mp

This guide is for existing `sampctl` projects that currently target **SA:MP** and want to switch to **open.mp**.

In `sampctl`, the migration is mostly:

- switch the **preset** / **compiler preset** to `openmp`
- switch the **runtime type** to `openmp` (so `config.json` is generated)
- review **plugins/components** and dependency URL schemes

## 1) Switch the package preset

In your `pawn.json` / `pawn.yaml`, set the package `preset` to `openmp`.

`pawn.json`:

```json
{
  "preset": "openmp",
  "entry": "gamemodes/main.pwn",
  "output": "gamemodes/main.amx"
}
```

This changes the default runtime/compiler choices that `sampctl` uses.

## 2) Switch the standard library dependencies (important)

Most existing SA:MP projects depend on the Pawn stdlib package:

- `pawn-lang/samp-stdlib`

For open.mp, you generally want the open.mp stdlib:

- `openmultiplayer/omp-stdlib`

There’s a common gotcha during migration: many community packages still depend (directly or transitively) on `pawn-lang/samp-stdlib` and/or `pawn-lang/pawn-stdlib`. If those resolve to the “normal” SA:MP-oriented versions, you can end up compiling against the wrong includes.

To avoid this, when migrating to open.mp, you should explicitly depend on the following three packages in your `dependencies` list:

```json
{
  "dependencies": [
    "openmultiplayer/omp-stdlib",
    "pawn-lang/samp-stdlib@open.mp",
    "pawn-lang/pawn-stdlib@open.mp"
  ]
}
```

When migrating an existing project:

- Remove plain `pawn-lang/samp-stdlib` (if present).
- Add the three dependencies above (or ensure they’re present).

Then refresh deps:

```bash
sampctl ensure
```

## 3) Switch the compiler preset (if you override it)

If you have a `build` / `builds` section that explicitly selects the compiler preset, update it.

Example `pawn.json`:

```json
{
  "preset": "openmp",
  "entry": "gamemodes/main.pwn",
  "output": "gamemodes/main.amx",
  "build": {
    "compiler": { "preset": "openmp" }
  }
}
```

If you *don’t* override the compiler in `build`, you can usually just rely on the package `preset`.

## 4) Switch the runtime type to open.mp

`sampctl` generates:

- `server.cfg` for SA:MP
- `config.json` for open.mp

Runtime type is determined by:

- `runtime.runtime_type` (explicit), or
- auto-detection from `runtime.version` (if it contains `openmp` or `open.mp`)

Minimal option (explicit):

```json
{
  "preset": "openmp",
  "entry": "gamemodes/main.pwn",
  "output": "gamemodes/main.amx",
  "runtime": {
    "runtime_type": "openmp",
    "rcon_password": "change-me",
    "port": 7777,
    "hostname": "My open.mp Server",
    "maxplayers": 50
  }
}
```

Alternative option (auto-detect via version string):

```json
{
  "runtime": {
    "version": "1.2.0-openmp"
  }
}
```

Note: `runtime.version` is also used for plugin/component **resource matching** (it must match the resource’s version string exactly to be selected).

## 5) Review runtime settings and config generation

- You still configure the server via `runtime` in `pawn.json` / `pawn.yaml`.
- For open.mp, `sampctl` maps common keys into `config.json` (and also supports open.mp-only sections like `network`, `logging`, `pawn`, etc).
- If you used `runtime.extra` for SA:MP `server.cfg` keys, for open.mp it is written into `config.json`.

See:

- [Runtime configuration guide](configuration.md)
- [Runtime configuration reference](runtime-configuration-reference.md)

## 6) Migrate plugin/component dependencies

`sampctl` supports different dependency URL schemes:

- `plugin://user/repo` downloads plugin binaries into `./plugins/`
- `component://user/repo` downloads open.mp components into `./components/`

For open.mp runs:

- `runtime.plugins` becomes `pawn.legacy_plugins` in `config.json`
- `runtime.components` becomes `pawn.components` in `config.json`

So when you switch runtimes:

- keep SA:MP-style plugins as `plugin://...` (if you still need them)
- prefer open.mp components as `component://...`

After changing dependencies, run:

```bash
sampctl ensure
```

## 7) Re-generate and run

From your project folder:

```bash
sampctl ensure
sampctl build
sampctl run
```

On open.mp, `sampctl run` will generate `config.json` and start the server.

## Optional: Keep both runtimes during transition

If you want to keep a SA:MP and an open.mp configuration side-by-side while you migrate, define multiple runtimes under `runtimes`:

```yaml
preset: openmp
entry: gamemodes/main.pwn
output: gamemodes/main.amx

runtimes:
  - name: samp
    runtime_type: samp
    port: 7777
  - name: openmp
    runtime_type: openmp
    port: 7778
```

Then select one at run time:

```bash
sampctl run samp
sampctl run openmp
```

## Troubleshooting

- **It still generates `server.cfg`**: set `runtime.runtime_type: openmp` or make sure `runtime.version` contains `openmp` / `open.mp`.
- **Plugin/component binaries aren’t downloading**: check the dependency scheme (`plugin://` vs `component://`) and make sure the dependency has `resources` metadata.
- **Wrong binary selected**: resource selection matches `runtime.version` by exact string; if there’s no exact match, `sampctl` falls back to an “empty version” resource for the same platform.
