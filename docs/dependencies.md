# Dependencies

List dependencies in `pawn.json` / `pawn.yaml` under `dependencies` or
`dev_dependencies`.

## Add and remove

```bash
sampctl install samp-incognito/samp-streamer-plugin:2.8.2
sampctl uninstall samp-incognito/samp-streamer-plugin
sampctl ensure
```

Install adds a declaration after installing its required dependencies. Uninstall
removes the declaration and updates an existing `pawn.lock` offline. Installed
files, including external and local paths, remain. Use `--dev` for development
dependencies. Ensure installs the declared dependencies.

A bare uninstall name matches a pinned declaration. An explicit tag, branch, or
commit must match it. Shared transitive pins remain. Older lockfiles without
relationship data may keep unused entries until the next successful ensure.

## Version pinning

- Tag: `user/repo:1.2.3`
- Branch: `user/repo@branch-name`
- Commit: `user/repo#<sha1>`

Pinned dependencies use the package definition from that revision. An unpinned
transitive dependency uses the project's explicit pin for that repo. Examples:

- `pawn-lang/YSI-Includes@5.x`
- `samp-incognito/samp-streamer-plugin:2.8.2`

## Resolve version conflicts

Add an `overrides` entry to the project manifest when a dependency requests a
version your project needs to replace. Run `sampctl ensure` to apply the
replacement and update an existing lockfile if it conflicts with the rule.
Use the original declaration when uninstalling or targeting an overridden
dependency for an update. A targeted update keeps a fixed tag chosen by the
override. Edit the replacement in the manifest to change that tag.
See [Project dependency overrides](packages.md#project-dependency-overrides)
for JSON and YAML examples.

## Dependency schemes

- `plugin://user/repo` downloads plugin binaries into `./plugins/`
- `component://user/repo` downloads open.mp components into `./components/`
- `includes://user/repo` adds include paths
- `filterscript://user/repo` fetches a filterscript's source repository

See [Dependency schemes](dependency-schemes.md) for details.

Plugins and components use their package's `resources` metadata to select a
GitHub release asset by `(platform, runtime.version)`. Sampctl installs the repo,
extracts the binaries into the directories above, and adds them to the runtime
config.

Resource versions must match `runtime.version` exactly. If none matches, sampctl
uses a resource for the same platform with an empty `version`. Versions containing
`openmp` or `open.mp` select open.mp. See [Plugin resources](plugin-resources.md)
for package author guidance.

## Install and refresh lockfile pins

- `sampctl install` preserves existing pins and saves `pawn.lock` when enabled.
- `sampctl ensure --no-lock` installs without using or writing `pawn.lock`.
- `sampctl ensure --lock-only` refreshes pins without installing dependencies.

See [Project lockfile](lockfiles.md) for what gets recorded and how to refresh pins.

## Failures and recovery

Dependency commands return a nonzero exit status on failure and report independent
errors together. Use `--verbose` for details or `sampctl check` to inspect local
state. For interrupted operations or cleanup failures, see
[Recover interrupted installations](troubleshooting.md#recover-interrupted-installations).
