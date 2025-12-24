# Running a server

When you run a project with `sampctl`, it starts a server using your compiled `.amx` as the gamemode.

## Prepare downloads (optional)

If you just want to download/refresh dependencies and runtime files without running:

```bash
sampctl ensure
```

This will also ensure the runtime binaries/plugins for the project are present.

## Run

```bash
sampctl run
```

![Run server](images/sampctl-server-run.gif)

`sampctl run` will:

- build your project if needed
- download server binaries if missing
- download any plugin/component dependencies
- generate `server.cfg` (SA:MP) or `config.json` (open.mp)
- start the server

Run in a Linux container:

```bash
sampctl run --container
```

See: [Containers (Docker)](containers.md)

## Select a runtime configuration

If your `pawn.json` / `pawn.yaml` has multiple entries under `runtimes`, you can pick one by name:

```bash
sampctl run <runtime-name>
```

See: [Runtime configuration](configuration.md)
