# Global configuration

`sampctl` has a global config file (and env vars) for settings like your default GitHub username and API tokens.

It also loads environment variables from a `.env` file in the directory you run it from.

## Config file location

The config file is stored in the `sampctl` cache directory (see `docs/cache.md`).

`sampctl` reads (first match wins):

- `config.json`
- `config.yaml`

If no config file exists yet, `sampctl` creates `config.json` with defaults.

## View and edit config

Show current config values:

```bash
sampctl config
```

Set a field:

```bash
sampctl config DefaultUser your-github-name
```

Field names use the internal names you see in `sampctl config` output (for example `DefaultUser`, `GitHubToken`).

## Useful settings

- `DefaultUser` (`SAMPCTL_DEFAULT_USER`): default GitHub username used by `sampctl init`
- `GitHubToken` (`SAMPCTL_GITHUB_TOKEN`): increases GitHub API rate limits (useful if `ensure` starts failing)
- `GitUsername` (`SAMPCTL_GIT_USERNAME`): git username for private repos
- `GitPassword` (`SAMPCTL_GIT_PASSWORD`): git password/token for private repos
- `HideVersionUpdateMessage` (`SAMPCTL_HIDE_VERSION_UPDATE_MESSAGE`): hide update reminder

## CI detection

If the `CI` environment variable is set (common on CI services), `sampctl` can change behavior for automation.
