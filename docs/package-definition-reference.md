# Package definition reference (`pawn.json` / `pawn.yaml`)

This page lists the common fields you can use in a `pawn.json` / `pawn.yaml` file.

For an introduction and a minimal example, see: [Packages](packages.md)

## Common fields

- `preset`: selects defaults for runtime/compiler. Common values: `samp`, `openmp`.
- `entry`: the `.pwn` file to compile.
- `output`: where the `.amx` output should be written.
- `runtime_dir`: where to install [local runtime files](#local-runtime-files).
- `dependencies`: packages to download for building/running.
- `dev_dependencies`: packages only needed for building/testing.
- `overrides`: optional map from dependency strings to replacement strings. See [Dependency overrides](#dependency-overrides).

## Dependency overrides

sampctl applies one substitution per declaration. If A maps to B and B maps
to C, a declaration of A resolves to B.

When a rule matches and selects a fixed tag, `sampctl ensure --update --force`
keeps that tag. Edit the replacement in the manifest to change it.

See [the PawnPlus example](packages.md#project-dependency-overrides) for JSON
and YAML syntax, exact matching, root scope, and precedence.

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

### Local runtime files

For SA-MP and open.mp, `runtime_dir` defaults to the package root. Relative paths
start at that root. Generated config files, server binaries, plugins, and
components go in the runtime directory.

When running locally with `runtime_dir` set, sampctl copies the build output
into the runtime's `gamemodes/` directory. For example, with
`runtime_dir: "server"`, `build/main.amx` is copied to
`server/gamemodes/main.amx` before the server starts.

## Build fields

- `build`: a single build configuration.
- `builds`: multiple build configurations (each should have a `name`).

Select a named build:

```bash
sampctl build <build-name>
```

See also: [Build configuration reference](build-configuration-reference.md)

## Advanced fields

- `local`: run locally instead of using a temporary runtime directory.
- `include_path`: for repos where the Pawn sources are in a subfolder.
- `resources`: advanced extra resources for a package (per-platform files, archives).
- `extract_ignore_patterns`: patterns to skip when extracting plugin archives.
- `experimental.build_file`: generate the [build include](#experimental-build-file-experimentalbuild_file).
- `contributors`, `website`: optional metadata (useful for published packages).
- `user`, `repo`: GitHub owner and repository name. Required when creating
  a GitHub release through [`sampctl release`](library-creator-guide.md#4-publish-a-release).

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

Include it in your Pawn source with `#include "sampctl_build_file.inc"`.

See also:

- [Library creator guide (includes + plugins)](library-creator-guide.md)
- [Plugin resources (for plugin library authors)](plugin-resources.md)
- [Dependency schemes](dependency-schemes.md)
