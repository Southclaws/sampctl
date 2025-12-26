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

### Recommended: declare `resources`

Add a `resources` section to your `pawn.json` describing how to find and extract plugin binaries from your GitHub release assets.

A minimal example for a plugin that ships two archives (Windows + Linux):

```json
{
  "entry": "test.pwn",
  "output": "test.amx",
  "dependencies": ["pawn-lang/samp-stdlib"],
  "resources": [
    {
      "name": "^my-plugin-(.*)\\.zip$",
      "platform": "windows",
      "archive": true,
      "includes": ["pawno/include"],
      "plugins": ["plugins/my-plugin.dll"]
    },
    {
      "name": "^my-plugin-(.*)\\.tar\\.gz$",
      "platform": "linux",
      "archive": true,
      "includes": ["pawno/include"],
      "plugins": ["plugins/my-plugin.so"]
    }
  ]
}
```

### Matching different release assets per runtime (SA:MP vs open.mp)

If you ship different binaries for SA:MP and open.mp, you typically need **two resources per platform**.

For example, if you publish both of these assets for the same plugin version:

- `my-plugin-1.2.3.zip` (SA:MP)
- `my-plugin-1.2.3-omp.zip` (open.mp)

…you can disambiguate them by using **different `resources[].name` patterns** and a **different `resources[].version`** for each runtime.

Example (Windows shown; mirror it for Linux):

```json
{
  "entry": "test.pwn",
  "output": "test.amx",
  "dependencies": ["pawn-lang/samp-stdlib"],
  "resources": [
    {
      "name": "^my-plugin-(.*)\\.zip$",
      "platform": "windows",
      "version": "0.3.7",
      "archive": true,
      "plugins": ["plugins/my-plugin.dll"],
      "includes": ["pawno/include"]
    },
    {
      "name": "^my-plugin-(.*)-omp\\.zip$",
      "platform": "windows",
      "version": "openmp",
      "archive": true,
      "plugins": ["components/my-plugin.dll"],
      "includes": ["pawno/include"]
    }
  ]
}
```

Important details:

- `resources[].version` is matched **exactly** against your runtime `version` string.
- `runtime_type` (SA:MP vs open.mp) does **not** affect resource selection; only `version` and `platform` do.
- If no exact `(platform, version)` match exists, `sampctl` falls back to the first resource with the same `platform` and an **empty** `resources[].version`.

So for open.mp, set your runtime `version` to something that includes `openmp` / `open.mp` (e.g. `v1.2.0-openmp`) and set the open.mp resource `version` to the **same** string.

### Resource `version` in practice

Use `resources[].version` when you ship:

- different binaries for `0.3.7` vs `0.3DL`, or
- different binaries for SA:MP vs open.mp.

If your binaries work for any server version for a platform, omit `resources[].version` and let the platform-only fallback handle it.

Conventions that make this work well:

- Release assets should have consistent names so the `name` regex matches.
- If the release asset is an archive, keep plugin files under `plugins/` and includes under `pawno/include/` inside that archive.
- If your plugin needs extra shared libraries, use `files` to map paths inside the archive to extraction paths.

See also: `docs/package-definition-reference.md` (the `resources` and `extract_ignore_patterns` fields).

### User install

Users can either:

- Add your repo as a normal dependency (recommended if you provide `resources`).
- Or use `plugin://user/repo` for “plugin-only” dependencies.

For open.mp components, users can use `component://user/repo`.

See: `docs/dependencies.md`.

## 7) Common pitfalls

- Setting `entry` to your `.inc` (libraries should compile via a `.pwn` entry).
- Forgetting to list a dependency your `.inc` relies on.
- Shipping plugin binaries without GitHub releases / stable asset naming.
- Conflicting `.inc` filenames across dependencies (avoid generic names; use include guards).
