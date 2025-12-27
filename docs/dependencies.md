# Dependencies

Dependencies are how `sampctl` downloads other packages your project needs.

They live in `pawn.json` / `pawn.yaml` under:

- `dependencies`
- `dev_dependencies`

## Add / remove

```bash
sampctl install <user/repo>
sampctl uninstall <user/repo>
sampctl ensure
```

## Version pinning

You can pin dependencies to specific versions.

- Tag: `user/repo:1.2.3`
- Branch: `user/repo@branch-name`
- Commit: `user/repo#<sha1>`

Examples:

- `pawn-lang/YSI-Includes@5.x`
- `samp-incognito/samp-streamer-plugin:2.8.2`

## Special schemes (plugins, components, includes)

Some dependencies are “installed” into special places instead of `./dependencies/`.

- `plugin://user/repo` downloads plugin binaries into `./plugins/`
- `component://user/repo` downloads open.mp components into `./components/`
- `includes://user/repo` adds additional include paths
- `filterscript://user/repo` installs a filterscript

See also: [Dependency schemes](dependency-schemes.md)

## Plugins/components: what actually happens

When a dependency is treated as a plugin/component, `sampctl`:

1) Ensures the repo (like a normal dependency).
2) Reads the dependency’s `resources` metadata.
3) Picks a single resource by matching `(platform, runtime.version)`.
4) Downloads the matching GitHub release asset and extracts binaries to:

- `./plugins/` for `plugin://` dependencies
- `./components/` for `component://` dependencies

Then it auto-adds the plugin/component name to the runtime config that is generated for the run.

### Runtime version strings (resource matching)

Resource selection is an **exact string match** against `runtime.version`.

- If there’s no exact match, `sampctl` falls back to a resource with the same `platform` and an empty resource `version`.
- open.mp detection is based on the runtime `version` string containing `openmp` or `open.mp`.

If you’re a plugin author, see: [Plugin resources](plugin-resources.md)

After changing dependencies:

```bash
sampctl ensure
```
