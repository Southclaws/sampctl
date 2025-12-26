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

## Plugin downloads fail with “check the package definition … against the release assets”

This usually means `sampctl` downloaded a release asset but extracted **zero matching files**.

Common causes:

- The plugin package `resources[].name` regex doesn’t match the actual asset filename.
- The selected `resources[]` entry doesn’t match your target `(platform, runtime.version)`.
- The `plugins` / `includes` / `files` paths don’t match the archive’s internal layout.

What to do:

- If you’re a **user**, try pinning the dependency to a known-good tag and re-run:

```bash
sampctl ensure --forceEnsure
```

- If you’re a **plugin author**, validate your `resources` config and see: [Plugin resources](plugin-resources.md)

## Plugin downloads match the wrong runtime version

Resource selection matches `resources[].version` to `runtime.version` by **exact string**.

- If you run open.mp, your runtime `version` should contain `openmp` or `open.mp`.
- If you ship separate assets (e.g. `-omp`), ensure the matching resource entry uses the same `version` string.

See: [Plugin resources](plugin-resources.md)

## “Works after I delete stuff” / forcing a clean run

If you want to force a full refresh (common for CI or debugging cache issues):

```bash
sampctl run --forceEnsure --forceBuild
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

## “fatal error 100: cannot read from file: <something>”

If you’re missing a different include:

- Confirm the dependency that provides it exists in `dependencies` (or `dev_dependencies` if it’s only for tests).
- Run `sampctl ensure`.
- If you’re using URL schemes (plugins/components/includes/filterscripts), see: [Dependency schemes](dependency-schemes.md)

## Include name conflicts

If two dependencies provide the same `.inc` filename, the compiler may pick the wrong one.

Common fixes:

- Prefer unique include filenames in libraries.
- Use include guards.
- Remove or pin the conflicting dependency.

## GitHub download/rate-limit errors (403)

If you hit GitHub API rate limits, set a token:

- Environment variable: `SAMPCTL_GITHUB_TOKEN`
- Or configure it globally: see [Global configuration](global-config.md)

## On Windows: double-click does nothing

Run `sampctl.exe` from a terminal (PowerShell or Command Prompt) so you can see any error output, and make sure its folder is on your `PATH`.
