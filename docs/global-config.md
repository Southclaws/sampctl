# Global configuration

Set defaults and GitHub credentials in the global config or through
environment variables. Sampctl also loads `.env` from your working directory.

## Config file location

The config file lives in the [sampctl cache directory](cache.md).

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
