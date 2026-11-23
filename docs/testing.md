# Testing Pawn code

Use a small test gamemode to exercise your Pawn code in the server runtime.

## Watch mode

Rebuild on file changes:

```bash
sampctl build --watch
```

Rebuild and restart the runtime on changes:

```bash
sampctl run --watch
```

## Install, rebuild, and run

Run this to install dependencies and rebuild before starting the server:

```bash
sampctl run --forceEnsure --forceBuild
```

## Runtime modes

Set `runtime.mode` in `pawn.json` or `pawn.yaml` to control when sampctl
stops the server:

- `server` keeps the server running by default.
- `main` stops the server at the end of startup, after `main()` runs.
- `y_testing` stops the server when y_testing prints its results and returns a
  nonzero exit status if any tests fail.

For a y_testing gamemode, set:

```json
{
  "runtime": {
    "mode": "y_testing"
  }
}
```
