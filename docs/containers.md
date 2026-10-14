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

The project is mounted at `/samp`, and the selected game port is published over
UDP. Images come from `ghcr.io/southclaws/sampctl` using the installed version.
The local sampctl cache is mounted with write access at `/root/.config/sampctl`
so dependency downloads, installation locks, and settings work inside the container.
