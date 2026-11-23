# Cache

Sampctl stores downloads, settings, and local templates in its app directory.
On Linux this is usually `~/.config/sampctl`. Other platforms use the OS local
app configuration directory.

## What to keep

| Path | Contents |
| --- | --- |
| `config.json` or `config.yaml` | Your [global settings](global-config.md), which may include credentials. |
| `dependency-overrides.json` | Your global dependency overrides. |
| `templates/<name>/` | Local [templates](packages.md#templates), including their editable manifests. |
| `remote-dependency-overrides.json` | Downloaded override list. Sampctl can fetch it again. |
| `compilers.json`, `runtimes.json`, `packages.json` | Cached download lists. Sampctl can refresh them. |
| `packages/<host>/<user>/<repo>/<branch>` | Cached Git checkouts. The branch directory is `default` when no branch is specified. |
| `pawn/<user>/<repo>/<version>` | Installed compilers and their supporting files. |
| `runtime_staging/<platform>/<version>/` | Staged runtime files used to install a server into a project. |

Older repository caches can be migrated to the host directory when their Git
origin matches the requested dependency.

## Clear cached downloads

Stop sampctl processes and finish pending recovery before removing cached
downloads. Keep your settings, overrides, and templates. Missing downloads
are fetched on demand.

Run `sampctl check` to inspect the cache. `sampctl check --fix` can repair
supported problems, including invalid download lists and some managed compiler
installations. Keep recovery files and backups until recovery finishes.
See [Recover interrupted installations](troubleshooting.md#recover-interrupted-installations).
