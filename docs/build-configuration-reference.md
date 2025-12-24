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

## Common fields

- `name`: build name (used with `sampctl build <name>`).
- `input`: override the input `.pwn` file for this build.
- `output`: override the output `.amx` file for this build.
- `workingDir`: working directory passed to the compiler.
- `includes`: extra include paths/files.
- `constants`: key/value constants passed to the compiler.

## Compiler selection

Under `compiler`:

- `preset`: `samp` or `openmp` (recommended).
- `site`/`user`/`repo`/`version`: use a compiler from a Git repo.
- `path`: use a locally installed compiler.

## Hooks

- `prebuild`: commands to run before compilation.
- `postbuild`: commands to run after compilation.

Note: `args` exists for older configs but `options` is preferred.
