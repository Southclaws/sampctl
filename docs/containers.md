# Containers (Docker)

## Run in a container

Start Docker, then run this from your Pawn project directory:

```bash
sampctl run --container
```

Sampctl runs the Linux server in the image matching your installed version
from `ghcr.io/southclaws/sampctl`. It mounts the project at `/samp`, publishes
the game port over UDP, and mounts the local cache at `/root/.config/sampctl`.

## Supported container platform

Release images target `linux/amd64`. The Linux Pawn compiler and game servers
include 32-bit x86 binaries, so on ARM, Docker must be able to run both amd64
and i386 workloads. Standalone ARM releases of sampctl are available.

The `sampctl run --container` command does not select an emulated platform.
Use Docker directly to request the amd64 image explicitly:

```bash
docker run --rm -it --platform linux/amd64 \
  --mount "type=bind,source=$PWD,target=/samp" \
  -e XDG_CONFIG_HOME=/root/.config \
  -p 7777:7777/udp \
  ghcr.io/southclaws/sampctl:latest run --dir /samp
```

Adjust the published port to match your project's runtime configuration. To
share a host cache, add a writable bind mount at `/root/.config/sampctl`.
