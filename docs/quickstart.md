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

Use `sampctl install` to add a library to your project. This keeps the standard
library dependencies selected by `sampctl init` for your preset.

```bash
sampctl install Southclaws/formatex
```

If you edit `pawn.json` or `pawn.yaml` by hand, add the package to the existing
`dependencies` list, then run:

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
