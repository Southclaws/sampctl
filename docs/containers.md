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

## Supported container platform

Release images target `linux/amd64`. The downloaded Linux Pawn compiler and
SA:MP/open.mp servers use 32-bit x86 binaries. The image retains `g++-multilib`
for their runtime libraries and for build hooks that compile with `g++ -m32`.
Git and CA certificates support dependency fetching and HTTPS downloads.

Native ARM64 container images are not published: installing ARM64 libraries or
an x86 cross-compiler would not enable execution of these downloaded binaries.
Standalone ARM Go releases remain available.

On an ARM host, the Docker engine must provide emulation for both amd64 image
processes and the downloaded i386 binaries. Emulating amd64 alone does not
guarantee that 32-bit toolchains and servers will run. Verify both workloads
with your engine before relying on them.

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
