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
- `experimental.build_file`: generate a build-time include file (`sampctl_build_file.inc`) containing build constants and git metadata (experimental).
- `contributors`, `website`: optional metadata (useful for published packages).

### Experimental build file (`experimental.build_file`)

When `experimental.build_file` is `true`, sampctl generates `sampctl_build_file.inc` in your project root before compilation and adds the project root to the include paths.

The generated file includes:

- Defines for `build.constants` values.
  - Numeric-looking values are emitted as numbers.
  - Other values are emitted as quoted strings with escaping.
  - Values starting with `$NAME` expand environment variable `NAME`.
- Git metadata defines if available (unless overridden by your constants): `SAMPCTL_BUILD_COMMIT`, `SAMPCTL_BUILD_COMMIT_SHORT`, `SAMPCTL_BUILD_BRANCH`.

Usage: set `experimental.build_file: true` in `pawn.json` / `pawn.yaml` and include it in your Pawn source with `#include "sampctl_build_file.inc"`.

See also:

- [Library creator guide (includes + plugins)](library-creator-guide.md)
- [Plugin resources (for plugin library authors)](plugin-resources.md)
- [Dependency schemes](dependency-schemes.md)
