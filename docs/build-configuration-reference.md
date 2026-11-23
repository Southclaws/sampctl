# Build configuration reference (`build` / `builds`)

Use `build` for one build configuration or `builds` for a list of named
configurations in `pawn.json` or `pawn.yaml`.

Select a named build:

```bash
sampctl build <build-name>
```

## Example

```json
{
  "entry": "gamemodes/main.pwn",
  "output": "gamemodes/main.amx",
  "build": {
    "compiler": { "preset": "samp" },
    "options": {
      "debug_level": 3
    }
  }
}
```

```yaml
entry: gamemodes/main.pwn
output: gamemodes/main.amx
build:
  compiler:
    preset: samp
  options:
    debug_level: 3
```

## Fields

- `name` (string): name to use with `sampctl build <build-name>`.
- `version` (string): compiler version. Prefer `compiler.version` in new manifests.
- `workingDir` (string): working directory for compilation.
- `input` (string): source file to compile.
- `output` (string): output `.amx` path.
- `includes` (string[]): include directories passed as `-i` flags.
- `constants` (map of strings): compiler definitions passed as `NAME=value`.
  See [the generated build include](package-definition-reference.md#experimental-build-file-experimentalbuild_file)
  for their use in Pawn source.

### Args and options

- `options` (object): [structured compiler options](#compiler-options-options).
- `args` (string[]): legacy raw compiler arguments. Sampctl uses these when
  `options` is absent.

### Build hooks

- `prebuild` runs commands before compilation.
- `postbuild` runs commands after compilation.

Each command is an array containing the executable and its arguments:

```yaml
build:
  prebuild:
    - ["echo", "hello"]
```

Replace legacy build `plugins` entries with hooks. Sampctl no longer runs
those entries.

## Compiler options (`options`)

These map to Pawn compiler flags:

- `debug_level` (int): `-d<level>`
- `require_semicolons` (bool): `-;+` / `-;-`
- `require_parentheses` (bool): `-(+` / `-(-`
- `require_escape_sequences` (bool): `-\\+` / `-\\-`
- `compatibility_mode` (bool): `-Z+` / `-Z-`
- `optimization_level` (int): `-O<level>`
- `show_listing` (bool): adds `-l` when enabled.
- `show_annotated_assembly` (bool): adds `-a` when enabled.
- `show_error_file` (string): `-e<filename>`
- `show_warnings` (bool): `-w+` / `-w-`
- `compact_encoding` (bool): `-C+` / `-C-`
- `tab_size` (int): `-t<spaces>`

## Compiler selection

- `preset` (string): `samp` or `openmp`.
- `path` (string): local path to a compiler. Use this field on its own.
- `site` / `user` / `repo` / `version` (strings): use a compiler from a Git repo.
