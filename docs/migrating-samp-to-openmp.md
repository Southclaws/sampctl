# Migrating from SA:MP to open.mp

## Change the preset and standard library

Set the top-level `preset` to `openmp`. Replace any existing SA:MP standard
library entries with the three below. The explicit branches also apply to
unpinned transitive requests for the same repositories.

```json
{
  "preset": "openmp",
  "dependencies": [
    "openmultiplayer/omp-stdlib@master",
    "pawn-lang/samp-stdlib@open.mp",
    "pawn-lang/pawn-stdlib@open.mp"
  ]
}
```

```yaml
preset: openmp
dependencies:
  - openmultiplayer/omp-stdlib@master
  - pawn-lang/samp-stdlib@open.mp
  - pawn-lang/pawn-stdlib@open.mp
```

Keep your existing `entry`, `output`, and other dependencies. If a build
configuration explicitly selects the SA:MP compiler preset, set
`compiler.preset` to `openmp` in that configuration.

## Update the runtime

If your manifest pins a SA:MP runtime version, change it to an open.mp
version. For example:

```json
{
  "runtime": {
    "runtime_type": "openmp",
    "version": "1.2.0-openmp"
  }
}
```

```yaml
runtime:
  runtime_type: openmp
  version: '1.2.0-openmp'
```

Without an explicit runtime version, the open.mp preset supplies the default.
Sampctl generates `config.json` for open.mp. Review any SA:MP-specific
`runtime.extra` settings before migrating. See the
[open.mp configuration mapping](runtime-configuration-reference.md#openmp-configjson-mapping).

## Review plugins and components

Keep `plugin://` for compatible legacy plugins. Use `component://` for open.mp
components so sampctl installs them in `components/`.

If sampctl selects the wrong asset, check the package's resource platform and
version. It matches the full runtime version string and falls back to an empty
resource version for the same platform. See [Plugin resources](plugin-resources.md#resource-selection).

## Build and run

```bash
sampctl ensure
sampctl build
sampctl run
```

## Keep both runtimes during migration

Define named runtimes if you need to switch between servers while testing:

```yaml
runtimes:
  - name: samp
    runtime_type: samp
    version: "0.3.7"
    port: 7777
  - name: openmp
    runtime_type: openmp
    version: "openmp"
    port: 7778
```

Select the runtime by name:

```bash
sampctl run samp
sampctl run openmp
```

Both runtimes use the project's build and dependencies unless you select a
different build or change the manifest. Check that your code and plugins work
with the runtime you select.
