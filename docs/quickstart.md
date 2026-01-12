# Quick start

This guide shows the most common workflow:

- Create a Pawn project (package)
- Add dependencies
- Build and run it (which starts a server)

## 1) Create a project

1) In your project folder, create a package definition (recommended):

```bash
sampctl init
```

![Initialise a project](images/sampctl-package-init.gif)

If you want open.mp instead of SA:MP (ideal for CI usage):

```bash
sampctl init --runtime openmp
```

2) Add dependencies (example):

`pawn.json`:

```json
{
  "entry": "test.pwn",
  "output": "gamemodes/test.amx",
  "dependencies": ["pawn-lang/samp-stdlib", "Southclaws/formatex"]
}
```

3) Install dependencies:

```bash
sampctl ensure
```

![Ensure dependencies](images/sampctl-package-ensure.gif)

4) Build and run:

```bash
sampctl build
sampctl run
```

More details: [Packages](packages.md)

More details: [Server](server.md) and [Runtime configuration](configuration.md)

Next:

- [Dependencies and version pinning](dependencies.md)
- [Testing (watch mode, runtime modes)](testing.md)
- [Containers (Docker)](containers.md)
- [Continuous integration (CI)](ci.md)
