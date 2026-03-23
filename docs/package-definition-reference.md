# Package definition reference (`pawn.json` / `pawn.yaml`)

This page lists the common fields you can use in a `pawn.json` / `pawn.yaml` file.

For an introduction and a minimal example, see: [Packages](packages.md)

## Common fields

- `preset`: selects defaults for runtime/compiler. Common values: `samp`, `openmp`.
- `entry`: the `.pwn` file to compile.
- `output`: where the `.amx` output should be written.
- `dependencies`: packages to download for building/running.
- `dev_dependencies`: packages only needed for building/testing.

## Runtime fields

- `runtime`: a single runtime configuration.
- `runtimes`: multiple runtime configurations (each should have a `name`).

Select a named runtime:

```bash
sampctl run <runtime-name>
```

See also:

- [Runtime configuration guide](configuration.md)
- [Runtime configuration reference](runtime-configuration-reference.md)

## Build fields

- `build`: a single build configuration.
- `builds`: multiple build configurations (each should have a `name`).

Select a named build:

```bash
sampctl build <build-name>
```

See also: [Build configuration reference](build-configuration-reference.md)

## Advanced fields

- `local`: if `true`, run/build inside your project folder instead of a temporary runtime folder.
- `include_path`: for repos where the Pawn sources are in a subfolder.
- `resources`: advanced extra resources for a package (per-platform files, archives).
- `extract_ignore_patterns`: patterns to skip when extracting plugin archives.
- `experimental.build_file`: control generation of the build-time include file (`sampctl_build_file.inc`) containing build constants and git metadata. This is enabled by default; set it to `false` to disable it.
- `contributors`, `website`: optional metadata (useful for published packages).

### Experimental build file (`experimental.build_file`)

By default, sampctl generates `sampctl_build_file.inc` in your project root before compilation.
Set `experimental.build_file: false` if you want to disable it.

The generated file includes:

- Built-in sampctl metadata defines:
  - `SAMPCTL_BUILD_FILE` as `1`.
  - `SAMPCTL_VERSION` with the running sampctl version, or `"unknown"` when using dev builds.
  - `SAMPCTL_PLATFORM` with the active target platform when available.
- Defines for `build.constants` values.
  - Numeric-looking values are emitted as numbers.
  - Other values are emitted as quoted strings with escaping.
  - Values starting with `$NAME` expand environment variable `NAME`.
- Git metadata defines if available (unless overridden by your constants): `SAMPCTL_BUILD_COMMIT`, `SAMPCTL_BUILD_COMMIT_SHORT`, `SAMPCTL_BUILD_BRANCH`.

Usage: include it in your Pawn source with `#include "sampctl_build_file.inc"`.
If you need to turn it off, set `experimental.build_file: false` in `pawn.json` / `pawn.yaml`.

See also:

- [Library creator guide (includes + plugins)](library-creator-guide.md)
- [Plugin resources (for plugin library authors)](plugin-resources.md)
- [Dependency schemes](dependency-schemes.md)
