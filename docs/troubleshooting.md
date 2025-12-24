# Troubleshooting

## “command not found: sampctl”

- Make sure `sampctl` is installed.
- Make sure the folder containing `sampctl` is on your `PATH`.
- On Linux/macOS, you may need to restart your terminal after changing `PATH`.

## “permission denied” on Linux

If you downloaded a binary manually, make it executable:

```bash
chmod +x ./sampctl
```

If you installed a `.deb`, re-run install with `sudo`.

## Plugins not downloading / wrong platform

If you’re targeting a different platform than the one you’re running on (for example building on Linux but preparing Windows files), use `--platform`.

Example:

```bash
sampctl ensure --platform windows
```

## Container mode issues

If you use `--container`, make sure Docker is installed and running.

Useful checks:

```bash
docker version
```

See: [Containers (Docker)](containers.md)

## “fatal error 100: cannot read from file: a_samp”

This usually means the SA:MP standard library isn’t available to the compiler.

- Make sure your `pawn.json` / `pawn.yaml` has `pawn-lang/samp-stdlib` in `dependencies`.
- Run `sampctl ensure` to download dependencies.

## GitHub download/rate-limit errors (403)

If you hit GitHub API rate limits, set a token:

- Environment variable: `SAMPCTL_GITHUB_TOKEN`
- Or configure it globally: see [Global configuration](global-config.md)

## On Windows: double-click does nothing

Run `sampctl.exe` from a terminal (PowerShell or Command Prompt) so you can see any error output, and make sure its folder is on your `PATH`.
