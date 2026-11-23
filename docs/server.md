# Running a server

## Run

```bash
sampctl run
```

![Run server](images/sampctl-server-run.gif)

`sampctl run` builds the project if the output is missing, prepares the
runtime, and starts the server. The compiled `.amx` is the default gamemode.
Run `sampctl ensure` to install dependencies and runtime files before starting
the server.

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
