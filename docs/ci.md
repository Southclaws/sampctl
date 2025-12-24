# Continuous integration (CI)

Because `sampctl` downloads the compiler and dependencies for you, it works well in CI.

## Recommended approach

- commit a working `pawn.json` / `pawn.yaml`
- run `sampctl ensure`
- run `sampctl build`
- if you have tests, run `sampctl run` in a test-friendly runtime mode

## Example: GitHub Actions (Ubuntu)

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Install sampctl
        run: |
          curl -fsSL https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-deb.sh | bash

      # Some Pawn toolchains/runtimes are 32-bit; you may need i386 support on Linux.
      - name: Install 32-bit support (optional)
        run: |
          sudo dpkg --add-architecture i386
          sudo apt-get update
          sudo apt-get install -y g++-multilib

      - name: Ensure
        run: sampctl ensure

      - name: Build
        run: sampctl build

      - name: Run (optional)
        run: sampctl run --forceBuild --forceEnsure
```

## Making tests fail the build

If you use y_testing, set `runtime.mode` to `y_testing` in `pawn.json`/`pawn.yaml` for your CI runtime.

See `docs/testing.md`.
