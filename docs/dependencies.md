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

After changing dependencies:

```bash
sampctl ensure
```
