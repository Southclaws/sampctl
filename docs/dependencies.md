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

Dependency commands stage managed installations and record recovery state in
`.sampctl-dependencies.json`. Before commit, a failure triggers restoration of
previous dependencies, the package definition, and `pawn.lock`. Output states
whether restoration finished. Valid shared caches remain.

The next dependency command recovers interrupted work. It restores missing
previous installations from verified backups, or preserves committed replacements
and finishes cleanup.
`sampctl check` reports pending recovery without changing project files;
`sampctl check --fix` performs the same safe recovery. Invalid metadata, changed
files, and ambiguous backups need manual inspection. Keep the record and backups
until installed files, the manifest, and pins agree. Unknown staging directories
remain.

Cleanup can fail after changes are saved. Output states that the operation
completed with cleanup pending and exits nonzero. Retry the command or run
`sampctl check --fix` to finish cleanup.

Runtime caches, binaries, and plugin files commit independently. Sampctl validates
file replacements before renaming them and records runtime ownership in
`.sampctl-runtime.json`, including external directories and `--no-lock` projects.
If an older installation has no recorded ownership, ensure its current version
once before upgrading.
Dependency rollback leaves these files in place. Interrupted provisioning can
leave a mix of complete previous and new runtime files; rerun `sampctl ensure`
to finish provisioning.

One dependency operation can change a project at a time; retry when it ends.
Recovery uses the same lock. The operating system releases it on process exit;
a leftover `.sampctl-dependencies.lock` file does not mean an operation is running.

Recovery requires renames within one filesystem. Directory changes are synced
where supported. On Windows, open files can prevent promotion or cleanup; close
the process using them and retry.
