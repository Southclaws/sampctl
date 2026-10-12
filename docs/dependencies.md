# Dependencies

List dependencies in `pawn.json` / `pawn.yaml` under `dependencies` or
`dev_dependencies`.

## Add and remove

```bash
sampctl install <user/repo>
sampctl uninstall <user/repo>
sampctl ensure
```

Install adds a declaration after installing its required dependencies. Uninstall
removes the declaration and updates an existing `pawn.lock` offline. Installed
files, including external and local paths, remain. Use `--dev` for development
dependencies. Ensure installs the declared dependencies.

A bare uninstall name matches a pinned declaration. An explicit tag, branch, or
commit must match it. Shared transitive pins remain; older lockfiles without
relationship data may keep unused entries until the next successful ensure.

## Version pinning

- Tag: `user/repo:1.2.3`
- Branch: `user/repo@branch-name`
- Commit: `user/repo#<sha1>`

Pinned dependencies use the package definition from that revision. Examples:

- `pawn-lang/YSI-Includes@5.x`
- `samp-incognito/samp-streamer-plugin:2.8.2`

## Dependency schemes

- `plugin://user/repo` downloads plugin binaries into `./plugins/`
- `component://user/repo` downloads open.mp components into `./components/`
- `includes://user/repo` adds include paths
- `filterscript://user/repo` installs a filterscript

See [Dependency schemes](dependency-schemes.md) for details.

Plugins and components use their package's `resources` metadata to select a
GitHub release asset by `(platform, runtime.version)`. Sampctl installs the repo,
extracts the binaries into the directories above, and adds them to the runtime
config.

Resource versions must match `runtime.version` exactly. If none matches, sampctl
uses a resource for the same platform with an empty `version`. Versions containing
`openmp` or `open.mp` select open.mp. See [Plugin resources](plugin-resources.md)
for package author guidance.

## Failures and recovery

Required dependency failures return a nonzero exit status. Sampctl reports
independent failures together. Use `--verbose` for details or `sampctl check`
to inspect local state.

Install and ensure prepare managed dependencies before replacing them. If
installation or saving project files fails, sampctl attempts to restore previous
dependencies, the package definition, and `pawn.lock`. Valid caches remain.
Rollback and cleanup failures return errors too; cleanup can fail after changes
have been saved.

With lockfile support enabled, success includes saving `pawn.lock`. Adding a
dependency preserves existing pins. `ensure --no-lock` leaves the lockfile alone;
`ensure --lock-only` refreshes it without installing dependencies.

Individual project files are written atomically. Runtime files may remain after
any failure. A process or machine failure can interrupt combined updates. After
an interruption, run `sampctl check`, inspect project files, then retry
`sampctl ensure`. Inspect any reported backup with `--verbose` before restoring
or deleting it.

Only one dependency operation can change a project at a time. Retry when the
current operation ends. A leftover `.sampctl-dependencies.lock` file does not
mean an operation is running.
