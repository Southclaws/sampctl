# Quick start

[Install sampctl](install.md) before starting.

## 1) Create a project

Run this in your project directory:

```bash
sampctl init
```

![Initialise a project](images/sampctl-package-init.gif)

The default preset is open.mp. For a SA:MP project, use:

```bash
sampctl init --preset samp
```

## 2) Add dependencies

List the packages you need under `dependencies` in `pawn.json` or
`pawn.yaml`. This example builds `test.pwn` into `gamemodes/test.amx`.

```json
{
  "entry": "test.pwn",
  "output": "gamemodes/test.amx",
  "dependencies": ["pawn-lang/samp-stdlib", "Southclaws/formatex"]
}
```

```yaml
entry: test.pwn
output: gamemodes/test.amx
dependencies:
  - pawn-lang/samp-stdlib
  - Southclaws/formatex
```

Run `ensure` after editing the manifest:

```bash
sampctl ensure
```

![Ensure dependencies](images/sampctl-package-ensure.gif)

## 3) Build and run

```bash
sampctl build
sampctl run
```

- [Manage Pawn packages](packages.md)
- [Run a server](server.md) and [configure the runtime](configuration.md)
- [Dependencies and version pinning](dependencies.md)
- [Testing (watch mode, runtime modes)](testing.md)
- [Containers (Docker)](containers.md)
- [Continuous integration (CI)](ci.md)
