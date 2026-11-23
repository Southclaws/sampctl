# Plugin resources

Declare `resources` in `pawn.json` or `pawn.yaml` so sampctl can download
your plugin binaries from GitHub releases.

For repository layout, tests, and release setup, see the
[library creator guide](library-creator-guide.md).

## Resource selection

Sampctl uses the first entry that matches the target platform and the exact
`runtime.version` string. If none matches, it uses the first entry for that
platform with an empty `version`. An empty runtime version defaults to
`0.3.7` for resource selection. The `runtime_type` field does not affect
which resource is selected.

## Resource fields

- `name` (string, required): a regular expression matching the GitHub release asset filename.
- `platform` (string, required): target platform, such as `windows` or `linux`.
- `version` (string, optional): runtime version string this resource belongs to.
- `archive` (bool, optional): whether the asset is an archive (`.zip` or `.tar.gz`).
- `includes` (string[], optional): directories inside the archive to extract (and treat as include roots).
- `plugins` (string[], optional): plugin binaries inside the archive to extract.
- `files` (map of strings, optional): archive paths mapped to destinations
  relative to the extraction directory.

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

```yaml
entry: test.pwn
output: test.amx
dependencies:
  - pawn-lang/samp-stdlib
resources:
  - name: '^my-plugin-(.*)\.zip$'
    platform: windows
    archive: true
    includes:
      - pawno/include
    plugins:
      - plugins/my-plugin.dll
  - name: '^my-plugin-(.*)\.tar\.gz$'
    platform: linux
    archive: true
    includes:
      - pawno/include
    plugins:
      - plugins/my-plugin.so
```

## Example: separate assets for SA:MP vs open.mp

Assume you publish both of these assets:

- `my-plugin-1.2.3.zip` (SA:MP)
- `my-plugin-1.2.3-omp.zip` (open.mp)

Use a separate resource for each runtime version. The SA:MP pattern below
matches the plain version number, so it won't also match the `-omp` archive.

```json
{
  "resources": [
    {
      "name": "^my-plugin-[0-9]+\\.[0-9]+\\.[0-9]+\\.zip$",
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

```yaml
resources:
  - name: '^my-plugin-[0-9]+\.[0-9]+\.[0-9]+\.zip$'
    platform: windows
    version: '0.3.7'
    archive: true
    plugins:
      - plugins/my-plugin.dll
    includes:
      - pawno/include
  - name: '^my-plugin-(.*)-omp\.zip$'
    platform: windows
    version: v1.2.0-openmp
    archive: true
    plugins:
      - components/my-plugin.dll
    includes:
      - pawno/include
```

`plugins` lists paths inside the archive. Use a `component://` dependency
to install the open.mp binary into `components/`.

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

```yaml
resources:
  - name: '^my-plugin-(.*)\.zip$'
    platform: windows
    archive: true
    plugins:
      - plugins/my-plugin.dll
    files:
      deps/libcurl.dll: libcurl.dll
```

## `extract_ignore_patterns`

To keep matching files that already exist when extracting dependency
resources, set `extract_ignore_patterns` in the consuming project's manifest:

```json
{
  "extract_ignore_patterns": ["server.cfg", "config.json"]
}
```

```yaml
extract_ignore_patterns:
  - server.cfg
  - config.json
```

See also:

- [Library creator guide (includes + plugins)](library-creator-guide.md)
- [Dependencies](dependencies.md)
- [Dependency schemes](dependency-schemes.md)
