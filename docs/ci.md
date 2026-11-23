# Continuous integration (CI)

Commit your `pawn.json` or `pawn.yaml` and the generated `pawn.lock`.
Run `sampctl ensure` in CI to install the locked dependencies, then
`sampctl build` to compile the project.

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

      # Some Pawn toolchains/runtimes are 32-bit. You may need i386 support on Linux.
      - name: Install 32-bit support (optional)
        run: |
          sudo dpkg --add-architecture i386
          sudo apt-get update
          sudo apt-get install -y g++-multilib

      - name: Ensure
        run: sampctl ensure

      - name: Build
        run: sampctl build
```

## Making tests fail the build

If your gamemode uses y_testing, set `runtime.mode` to `y_testing` and add
this step after the build:

```yaml
      - name: Run tests
        run: sampctl run
```

Sampctl exits with a nonzero status if y_testing reports a failure. See
[Runtime modes](testing.md#runtime-modes) for the other options.
