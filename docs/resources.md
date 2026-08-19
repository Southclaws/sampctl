# Resources (plugins, components, includes, and files)

A package resource is a release asset published by a dependency. Resources are useful when a package contains more than Pawn source files: compiler includes, server plugins, open.mp components, configuration files, templates, data files, or other files required at runtime.

Resources are declared in the dependency's `pawn.json` or `pawn.yaml` file. The dependency must publish a matching release asset, and the `resource` entry tells `sampctl` which asset to select and which files to extract from it.

## A resource at a glance

A resource entry describes one release asset for one target platform and, optionally, one runtime version:

```json
{
  "resources": [
    {
      "name": "^example-.*\\.zip$",
      "platform": "windows",
      "version": "0.3.7",
      "archive": true,
      "includes": ["release/include/.*\\.inc$"],
      "plugins": ["release/plugins/example.dll"],
      "files": {
        "release/config/example.json": "config/example.json",
        "release/data/lookup.bin": "data/lookup.bin"
      }
    }
  ]
}
```

The fields have the following roles:

| Field | Purpose |
| --- | --- |
| `name` | Regular expression matched against the release asset filename. |
| `platform` | Target platform, such as `windows` or `linux`. This field is required. |
| `version` | Runtime version associated with the asset. An empty value acts as the platform fallback. |
| `archive` | Set to `true` when the selected asset is a `.zip` or `.tar.gz` archive. |
| `includes` | Archive paths that provide Pawn include files. These files are extracted into the resource include directory. |
| `plugins` | Archive paths that provide server plugin binaries. These files are installed into the runtime plugin directory. For `component://` dependencies, the same resource mechanism installs the binaries as open.mp components. |
| `files` | A map from archive paths to destination paths for auxiliary files. These files are extracted but are not automatically loaded as plugins or components. |

The `name` field is a regular expression, not a literal filename. Escape dots and other regular-expression characters when necessary. Archive paths in `includes`, `plugins`, and the keys of `files` are also matched as regular expressions, which makes patterns useful for release archives that contain versioned or nested directories.

## Choosing a resource

When a dependency has several resources, `sampctl` first looks for an entry whose `platform` and `version` both match the active runtime. If no exact version match exists, it uses the first entry for that platform whose `version` is empty.

This makes it possible to publish a runtime-specific asset and still provide a default asset for the platform:

```json
{
  "resources": [
    {
      "name": "^example-(.*)-omp\\.zip$",
      "platform": "windows",
      "version": "v1.0.0-openmp",
      "archive": true,
      "plugins": ["plugins/example.dll"]
    },
    {
      "name": "^example-(.*)\\.zip$",
      "platform": "windows",
      "version": "",
      "archive": true,
      "plugins": ["plugins/example.dll"]
    }
  ]
}
```

Keep the default entry after more specific entries. A resource with an empty `version` is a fallback, not a wildcard that is selected before an exact version match.

## Resources that are not plugins

Use `files` when an archive contains files that must be installed but must not be added to the server's plugin or component list. The map key identifies a path inside the archive, and the value is the destination path relative to the active `sampctl` working directory.

For example, a package can ship a configuration template and a data file without treating either one as a plugin:

```json
{
  "resources": [
    {
      "name": "^example-tools-.*\\.tar\\.gz$",
      "platform": "linux",
      "archive": true,
      "files": {
        "release/templates/server.ini": "templates/server.ini",
        "release/data/commands.dat": "data/commands.dat"
      }
    }
  ]
}
```

The release archive might have this layout:

```text
release/
├── data/
│   └── commands.dat
└── templates/
    └── server.ini
```

After the resource is ensured, the files are available at:

```text
templates/server.ini
data/commands.dat
```

The destination directories are created as part of extraction. A `files` entry does not add a file to `plugins`, `components`, or the generated runtime plugin list; it only installs the file at the requested destination.

For a file that should be loaded by the server as a plugin or an open.mp component, use `plugins` instead. For a Pawn include, use `includes` so that the compiler can find it during a build.

## Includes and plugins in the same archive

A single archive can provide several types of resource. The following example installs an include, a plugin, and an auxiliary license file from one release asset:

```json
{
  "resources": [
    {
      "name": "^example-1\\.2\\.3-linux\\.tar\\.gz$",
      "platform": "linux",
      "version": "0.3.7",
      "archive": true,
      "includes": ["package/include/.*\\.inc$"],
      "plugins": ["package/plugins/example.so"],
      "files": {
        "package/LICENSE": "licenses/example.LICENSE"
      }
    }
  ]
}
```

The include files are added to the compiler's include search paths. The plugin is installed into the runtime's plugin directory and is included in the generated plugin configuration. The license is installed as an ordinary file and is not loaded by the server.

## Archives and single-file resources

Set `archive: true` for `.zip` and `.tar.gz` release assets that contain multiple files. The `includes`, `plugins`, and `files` fields are meaningful for archive entries.

A resource with `archive: false` represents a single release file. The current single-file path is intended for plugin binaries: `sampctl` copies the file into the requested plugin destination. Use an archive with `files` when you need to install arbitrary auxiliary files or more than one file from a release.

## Practical constraints

Resources are selected by one platform/version entry and one matching release asset. They are not a general-purpose package manager for copying arbitrary local directories. In particular:

- `files` applies to archive entries and maps matching source paths to destination paths.
- `files` installs files but does not load them as plugins or components.
- `includes` makes matching archive files available to the compiler; it does not add server plugins.
- `plugins` identifies binaries that should be loaded by the runtime. For open.mp component dependencies, the same binaries are installed under the component directory.
- A resource must provide a valid `name` regular expression and a target `platform`.
- Release archive paths should be written with forward slashes and should not contain paths that escape the working directory.

If an archive contains user-owned files such as `config.json`, consider using `extract_ignore_patterns` in the package definition to avoid overwriting matching existing files during extraction.

See also:

- [Package definition reference](package-definition-reference.md)
- [Dependency schemes](dependency-schemes.md)
- [Plugin resources](plugin-resources.md)
- [Library creator guide](library-creator-guide.md)
