# Create a Pawn library

Add a `pawn.json` or `pawn.yaml` so projects can install your library with
sampctl. If you also ship server binaries, describe them in `resources`.

## 1) Recommended repository layout

- Put your public include at the repo root (recommended), e.g. `mylib.inc` so users can do `#include <mylib>`.
- Keep a small entry script for compilation/tests, e.g. `test.pwn` (or `tests/test.pwn`).
- Optional: keep examples under `examples/`.

Example:

```
.
├── pawn.json
├── mylib.inc
├── test.pwn
└── README.md
```

If you must keep sources in a subfolder (common for plugins that store C/C++ in the repo root), set `include_path` in `pawn.json`.

## 2) Create the package definition

From your repo folder:

```bash
sampctl init
```

Set `entry` to a test or demo `.pwn` that includes your library.

Use this manifest for an includes-only library.

```json
{
  "entry": "test.pwn",
  "output": "test.amx",
  "dependencies": ["pawn-lang/samp-stdlib"],
  "runtime": {
    "mode": "main"
  }
}
```

```yaml
entry: test.pwn
output: test.amx
dependencies:
  - pawn-lang/samp-stdlib
runtime:
  mode: main
```

List the packages needed by your public includes in `dependencies`. Put
test-only packages, such as YSI when used for `y_testing`, in `dev_dependencies`.
Include every header your code uses, including `a_samp` if needed.

## 3) Test the library

### Fast compilation checks

```bash
sampctl build
```

See [Testing Pawn code](testing.md) for watch mode and `y_testing`.

### Run an interactive demo

Set `entry` to a small gamemode and use `runtime.mode: server` to keep it
running. Users can clone the repository and start the demo:

```bash
sampctl ensure
sampctl run
```

## 4) Publish a release

Publish version tags such as `1.2.3` so consumers can pin a release.

Before releasing:

- Run `sampctl ensure` and `sampctl build` against your test script. Commit
  the changes you want to release and start with a clean worktree.
- Check that you can push to the repository's Git remote.
- For a GitHub release, set `user` and `repo` in your manifest and configure
  a GitHub token with write access to that repository.

```bash
sampctl release
```

The wizard asks for a version and offers to generate a version include.
It creates a release commit and pushes the branch and tag. If you also
choose a GitHub release, sampctl creates a draft and attempts to upload a
ZIP of the package files. Review and publish the draft on GitHub. For a
plugin, attach the platform binaries expected by your resource definitions.
If draft creation or upload fails, check the remote tag before retrying.
The Git push happens before either GitHub step.

## 5) Package plugin binaries

Use stable release asset names and declare the binaries in
[`resources`](plugin-resources.md). Consumers can install them with
[`plugin://` or `component://`](dependency-schemes.md).

Choose distinct names for public `.inc` files to avoid collisions with
other libraries. Use include guards to prevent repeated inclusion.
