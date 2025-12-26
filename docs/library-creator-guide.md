# Library creator guide (includes + plugins)

This guide is for people *creating* libraries intended to be consumed via `sampctl`.

- **Includes-only library**: ships Pawn `.inc` (and maybe `.pwn`) sources only.
- **Plugin library**: ships Pawn includes *and* server plugin binaries (`.so`/`.dll`).

`sampctl` treats libraries the same way it treats gamemodes: as a **package** described by `pawn.json` / `pawn.yaml`.

## 1) Recommended repository layout

Keep it boring and predictable:

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

For a library, your `entry` should be a **test / demo script**, not the `.inc` itself.

Minimal `pawn.json` for an includes-only library:

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

Notes:

- `dependencies` should contain everything your library needs to compile *when the user only includes your `.inc`*.
- Prefer `dev_dependencies` for dependencies only used by your tests/demos (e.g. `pawn-lang/YSI-Includes` for `y_testing`).

## 3) “Batteries included” include style

Write libraries as if the user has **zero other packages installed**.

- Include what you use (yes, even `a_samp`).
- This makes your library compile in isolation and avoids “works on my machine” dependency drift.

## 4) Testing a library

### Fast compilation checks

```bash
sampctl build
```

### Watch mode

```bash
sampctl build --watch
sampctl run --watch
```

### Unit tests with `y_testing`

Set the runtime mode:

```json
{
  "runtime": {
    "mode": "y_testing"
  }
}
```

Then:

```bash
sampctl run --forceEnsure --forceBuild
```

### “Demo tests”

A very effective pattern is making your `entry` a tiny gamemode that demonstrates the library in-game. Users can clone your repo and run:

```bash
sampctl ensure
sampctl run
```

## 5) Versioning and releasing

`sampctl` supports version pinning in dependencies (tags/branches/commits), so library authors should publish **version tags**.

Recommended:

- Use Semantic Versioning tags like `1.2.3` (or your project’s established convention).
- Create GitHub releases for tagged versions.

To create a versioned release interactively:

```bash
sampctl release
```

## 6) Plugin libraries (includes + binaries)

If your library depends on a server plugin, you have two goals:

1) Make it easy for users to get the `.inc`.
2) Make it easy for `sampctl` to download the correct `.dll`/`.so` for the user’s platform.

### Plugin packaging (single source of truth)

Plugin libraries should declare `resources` in `pawn.json` so `sampctl` can download the right binaries from GitHub release assets.

To avoid repeating (and keeping this guide focused on getting started), the detailed `resources` reference and examples live here:

- [Plugin resources (for plugin library authors)](plugin-resources.md)

Related user-facing docs:

- [Dependency schemes (plugins, components, includes)](dependency-schemes.md)
- [Dependencies](dependencies.md)

## 7) Common pitfalls

- Setting `entry` to your `.inc` (libraries should compile via a `.pwn` entry).
- Forgetting to list a dependency your `.inc` relies on.
- Shipping plugin binaries without GitHub releases / stable asset naming.
- Conflicting `.inc` filenames across dependencies (avoid generic names; use include guards).
