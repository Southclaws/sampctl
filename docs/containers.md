# Containers (Docker)

You can run your project in a Linux environment using Docker.

This is useful if you:

- use macOS and want to run a Linux runtime
- want a consistent environment in CI
- want to test Linux compatibility

## Requirements

- Docker installed and running

## Run in a container

From a project folder that contains `pawn.json` or `pawn.yaml`:

```bash
sampctl run --container
```

In container mode, `sampctl` runs the runtime as Linux.
