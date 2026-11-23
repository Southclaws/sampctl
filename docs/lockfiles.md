# Project lockfile

Commit `pawn.lock` alongside `pawn.json` or `pawn.yaml` so other developers
and CI can install the same dependency revisions. Edit the manifest to change
what the project requests. Let sampctl write the lockfile.

## What it records

- **Dependencies:** requested constraints, resolved versions and Git commits,
  plus relationships between direct and transitive dependencies.
- **Runtime:** the selected version and platform, with file hashes and other
  metadata for managed runtime files when available.
- **Build:** the compiler configuration, entry, output, and output hash recorded
  after a successful build when available.

Use the same sampctl version, target platform, and build configuration in CI.
See [Pin the sampctl version](ci.md#pin-the-sampctl-version).

## Install or refresh pins

| Command | Effect |
| --- | --- |
| `sampctl ensure` | Installs dependencies and runtime files, reusing compatible pins. Creates or updates the lockfile as needed. |
| `sampctl ensure --update` | Refreshes unpinned and `latest` dependencies. |
| `sampctl ensure --update --force` | Also refreshes explicit tag pins. Fixed tags selected by project overrides stay pinned. |
| `sampctl ensure --update pawn-lang/samp-stdlib` | Targets one dependency for update. |
| `sampctl ensure --lock-only` | Resolves and saves pins without installing dependencies or runtime files. Combine with `--update` to refresh pins before installing. |
| `sampctl ensure --no-lock` | Resolves and installs without reading or writing `pawn.lock`. |

`sampctl install` preserves compatible pins when adding a dependency.
`sampctl uninstall` updates an existing lockfile when removing a declaration.
A successful `sampctl build` can update the build record. Use
`sampctl build --no-lock` to skip lockfile support for that build.

Run `sampctl check` to diagnose lockfile problems. After changing a declaration
or [project override](packages.md#project-dependency-overrides), run
`sampctl ensure` and review the manifest and lockfile diff together.
