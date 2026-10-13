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

Pinned dependencies use the package definition from that revision. An unpinned
transitive dependency uses the project's explicit pin for that repo. Examples:

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

Failures return a nonzero exit status, with independent errors reported together.
Use `--verbose` for details or `sampctl check` to inspect local state.

Install preserves existing pins and saves `pawn.lock` when enabled.
`ensure --no-lock` leaves it alone; `ensure --lock-only` refreshes pins without
installing dependencies.

Dependency commands record staged changes in `.sampctl-dependencies.json`. Before
commit, failures trigger an attempt to restore previous installations, the
manifest, and `pawn.lock`. Output reports incomplete recovery. Valid shared caches
remain.

The next dependency command recovers pending work. `sampctl check` reports it
read-only; `sampctl check --fix` runs the same recovery. Saved changes remain
installed if cleanup fails; the command exits nonzero. Retry to finish cleanup.

Invalid metadata, changed files, or ambiguous backups need manual inspection.
Keep the record and backups until installed files, the manifest, and pins agree.
Unknown staging directories remain.

Runtime caches and files commit independently using validated replacements.
`.sampctl-runtime.json` records binary ownership, including external directories
and `--no-lock` projects. For older untracked installations, ensure the current
version before upgrading. Dependency rollback leaves runtime files in place;
interruptions can leave complete files from different versions. Rerun
`sampctl ensure` to finish provisioning.

Recovery uses the existing project lock; retry when another operation finishes.
The OS releases the lock on exit, so a leftover `.sampctl-dependencies.lock` file
does not mean an operation is active. Renames require one filesystem; directory
changes are synced where supported. On Windows, close files blocking promotion
or cleanup, then retry.
