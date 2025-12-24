# Testing Pawn code

`sampctl` is designed to make it easy to build and run small test gamemodes for libraries.

## Watch mode

Rebuild on file changes:

```bash
sampctl build --watch
```

Rebuild + restart the runtime on changes:

```bash
sampctl run --watch
```

## Forcing a clean run

If you want a “do everything” command (useful for demos and CI):

```bash
sampctl run --forceEnsure --forceBuild
```

## Runtime modes

Runtime modes change how `sampctl` decides when to stop the server process.

Set it in `pawn.json` / `pawn.yaml` under `runtime.mode`:

- `server`: normal server behavior (default)
- `main`: exit after `main()` finishes (good for quick checks)
- `y_testing`: exit with success/failure based on y_testing results (good for CI)

Example:

```json
{
  "runtime": {
    "mode": "y_testing"
  }
}
```
