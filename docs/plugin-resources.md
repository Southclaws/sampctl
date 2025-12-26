# Plugin resources (for plugin library authors)

This page is for **plugin library** maintainers: libraries that ship Pawn includes *and* server binaries (`.dll` / `.so`) via GitHub releases.

If you’re starting a new library repo (layout, tests, releases), see: [Library creator guide (includes + plugins)](library-creator-guide.md)

To make `sampctl` automatically download and install your binaries, declare `resources` in your `pawn.json` / `pawn.yaml`.

## How `sampctl` selects a resource (important)

When installing plugin binaries from a package:

- `sampctl` selects **one** `resources[]` entry by matching:
  - `platform` (must match the target runtime platform)
  - `version` (must exactly match the runtime `version` string)
- If there is no exact version match, it falls back to the first resource where:
  - `platform` matches
  - `version` is empty

So:

- Use `resources[].version` to ship different binaries for different runtimes.
- `resources[].version` is compared as a **raw string** (no semver parsing).

## Resource fields

Each entry in `resources` is a Resource object:

- `name` (string, required): a regular expression matching the GitHub release asset filename.
- `platform` (`"windows"` or `"linux"`, required): target platform.
- `version` (string, optional): runtime version string this resource belongs to.
- `archive` (bool, optional): whether the asset is an archive (`.zip` or `.tar.gz`).
- `includes` (string[], optional): directories inside the archive to extract (and treat as include roots).
- `plugins` (string[], optional): plugin binaries inside the archive to extract.
- `files` (map string->string, optional): extra files inside the archive to extract; values are extraction paths relative to the runtime root.

Related advanced field:

- `extract_ignore_patterns` (string[], optional): patterns of files that should not be overwritten during extraction. This is meant for users so they can avoid overwriting their own config files.

## Example: simple per-platform archives

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

## Example: separate assets for SA:MP vs open.mp

Assume you publish both of these assets:

- `my-plugin-1.2.3.zip` (SA:MP)
- `my-plugin-1.2.3-omp.zip` (open.mp)

You can model them as two resources per platform, distinguished by `name` and `version`.

```json
{
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
      "version": "v1.2.0-openmp",
      "archive": true,
      "plugins": ["components/my-plugin.dll"],
      "includes": ["pawno/include"]
    }
  ]
}
```

Notes:

- `runtime_type` does not affect resource selection; only `runtime.version` and `platform` do.
- open.mp detection is based on the runtime `version` string containing `openmp` or `open.mp`.
- The `plugins` paths are paths **inside the archive**; `sampctl` extracts them into the correct runtime folder.

## Extra files example (`files`)

If your plugin needs extra DLLs/SOs:

```json
{
  "resources": [
    {
      "name": "^my-plugin-(.*)\\.zip$",
      "platform": "windows",
      "archive": true,
      "plugins": ["plugins/my-plugin.dll"],
      "files": {
        "deps/libcurl.dll": "libcurl.dll"
      }
    }
  ]
}
```

## `extract_ignore_patterns`

If you extract archives that contain files that might overwrite user files, set:

```json
{
  "extract_ignore_patterns": ["server.cfg", "config.json"]
}
```

This will skip overwriting existing matching files during archive extraction.

See also:

- [Library creator guide (includes + plugins)](library-creator-guide.md)
- [Dependencies](dependencies.md)
- [Dependency schemes](dependency-schemes.md)
