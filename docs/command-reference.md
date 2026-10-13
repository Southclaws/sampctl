# Command reference

This page is a quick map of what commands exist and what they’re for.

For full help (including flags), use:

```bash
sampctl help
sampctl <command> --help
```

## Server commands

There are no separate “server-only” commands in the current CLI.

You run a server by running a Pawn project:

- `sampctl ensure`: download/update dependencies and runtime files
- `sampctl run [runtime-name]`: build (if needed) and run your project as a server

## Package commands

- `sampctl init`: create a new project (`pawn.json` / `pawn.yaml` + folder layout)
- `sampctl install <dep...>`: add dependency (writes to `pawn.json` / `pawn.yaml`)
- `sampctl uninstall <dep...>`: remove dependency
- `sampctl ensure`: ensure dependencies (and runtime files) are present
- `sampctl build [build-name]`: compile the project
- `sampctl run [runtime-name]`: compile (if needed) and run in a runtime
- `sampctl get <user/repo>`: clone a GitHub package and ensure it
- `sampctl release`: create a versioned package release

## Project diagnostics

```bash
sampctl check
sampctl check --fix
sampctl check --verbose
sampctl check --dir path/to/project
```

`check` validates settings, file paths, Git availability, dependencies, optional `pawn.lock`, compiler files, and cached download lists. It also reports interrupted dependency operations and pending cleanup. It searches the selected directory and its parents; outside a project, only global checks run.

Checks are read-only, with no downloads or compiler execution. Problems appear first: ✓ passed, ✗ failed, ! warning. Redirected output, `--bare`, and `NO_COLOR` disable colour. Use `--verbose` for details; missing prerequisites show “Skipped”.

`--fix` removes invalid download lists, restores managed compiler execute permissions, and recovers recorded dependency operations under the project lock. Recovery may restore the captured project configuration and lockfile or finish committed cleanup. Each repair is verified; custom compilers remain unchanged.

Invalid recovery metadata, ambiguous backups, symlinked cache paths, and hardlinked compilers need manual inspection. Archive integrity and unrecorded temporary files aren't repaired.

Exit codes: `0` for success, `1` for unresolved problems, `2` for command errors or cancellation. Informational warnings and skipped checks don't cause failure.

## Templates

- `sampctl template make`: create a template from a package
- `sampctl template build`: build a file with a template
- `sampctl template run`: run a file with a template

## Other

- `sampctl version`: show the sampctl version
- `sampctl config`: view/change global sampctl config
- `sampctl compiler list`: list compiler configurations
- `sampctl completion`: print shell completion script
- `sampctl docs`: print auto-generated markdown docs (advanced)
