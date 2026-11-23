# Troubleshooting

## Diagnose a project

Run `sampctl check` and follow the reported actions. Add `--verbose` for
details or `--fix` for supported repairs. See
[Project diagnostics](command-reference.md#project-diagnostics) for coverage
and exit codes.

## Recover interrupted installations

Run `sampctl check` to see what needs recovery, then `sampctl check --fix` to
resume recorded work. The next dependency command also resumes dependency
recovery, and the next `ensure` resumes runtime recovery.

- Dependency operations use `.sampctl-dependencies.json` to recover installed
  files, the manifest, and `pawn.lock`. Valid shared caches remain available.
- Runtime upgrades finish independently. The `.sampctl-runtime.json` file
  records ownership even for external runtime directories and projects using
  `--no-lock`. For an older untracked runtime, ensure its current version before
  upgrading.
- If the command reports a cleanup failure after saving changes, retry to finish
  cleanup. The installed changes are already in place, even though the command
  returned a nonzero status.

Keep recovery files and backups if sampctl reports invalid metadata, changed
files, or ambiguous backups. Inspect unrecognized staging directories before
removing them.

If another dependency command holds the project lock, wait for it to finish.
A leftover `.sampctl-dependencies.lock` file can remain after the OS releases
the lock. Recovery renames require paths on the same filesystem. On Windows,
close any files that prevent sampctl from replacing or removing them, then retry.

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

## Plugins download for the wrong platform

Use `--platform` to prepare files for another operating system. For example,
run this on Linux to download Windows plugins:

```bash
sampctl ensure --platform windows
```

## Missing plugin files after extraction

If a release archive contains no matching files, pin the plugin to a working
tag in your manifest and run `sampctl ensure`.

If you maintain the plugin, check the [resource definition](plugin-resources.md):

- Check that `resources[].name` matches the release asset filename.
- Check the resource for the target platform and runtime version.
- Check `plugins`, `includes`, and `files` against the paths inside the archive.

## Plugin downloads match the wrong runtime version

When a resource specifies a version, `resources[].version` must match
`runtime.version` exactly. If you maintain separate releases for SA-MP and
open.mp, check the version on each matching resource entry.
See [Plugin resources](plugin-resources.md).

## Rebuild and check dependencies before running

See [Install, rebuild, and run](testing.md#install-rebuild-and-run).

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
