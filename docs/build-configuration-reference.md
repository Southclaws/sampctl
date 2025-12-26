# Build configuration reference (`build` / `builds`)

Build configuration lives in your `pawn.json` / `pawn.yaml` under `build` (single) or `builds` (multiple).

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

## Fields

All fields below are supported under a build entry (e.g. `builds.<name>`).

- `name` (string): build name (usually set implicitly by the `builds` map key).
- `version` (string): optional version label.
- `workingDir` (string): working directory for compilation.
- `input` (string): source file to compile.
- `output` (string): output `.amx` path.
- `includes` (string[]): include directories passed as `-i` flags.
- `constants` (object map): constant definitions passed as `-D` flags (key/value).

### Args and options

You can provide raw arguments and/or structured options:

- `args` (string[]): raw arguments passed to the compiler (**deprecated**, prefer `options`).
- `options` (object): structured compiler options (see below).

### Plugins (pre-compile commands)

- `plugins` (array of string arrays): commands to run before compilation.

Example:

```yaml
build:
  plugins:
    - ["echo", "hello"]
```

### Compiler selection

- `compiler` (string or object):
  - string: a preset name (e.g. `samp`, `openmp`).
  - object: a compiler configuration (see **Compiler config** below).

### Hooks

- `prebuild` (array of string arrays): commands run before compilation.
- `postbuild` (array of string arrays): commands run after compilation.

## Compiler options (`options`)

These map to Pawn compiler flags:

- `debug_level` (int): `-d<level>`
- `require_semicolons` (bool): `-;+` / `-;-`
- `require_parentheses` (bool): `-(+` / `-(-`
- `require_escape_sequences` (bool): `-\\+` / `-\\-`
- `compatibility_mode` (bool): `-Z+` / `-Z-`
- `optimization_level` (int): `-O<level>`
- `show_listing` (bool): `-l` / `-l-`
- `show_annotated_assembly` (bool): `-a` / `-a-`
- `show_error_file` (string): `-e<filename>`
- `show_warnings` (bool): `-w+` / `-w-`
- `compact_encoding` (bool): `-C+` / `-C-`
- `tab_size` (int): `-t<spaces>`

## Compiler config (`compiler` as an object)

If `compiler` is an object, these fields are supported:

- `preset` (string): `samp` or `openmp`.
- `path` (string): local path to a compiler.
- `site` / `user` / `repo` / `version` (strings): use a compiler from a Git repo.

## Compiler presets

`sampctl` ships with built-in compiler presets: `samp` and `openmp`.
