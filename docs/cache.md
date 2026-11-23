# Cache

Sampctl caches dependency, runtime, plugin, and compiler downloads.

## Where is the cache?

On Linux, the cache is usually `~/.config/sampctl`. Other platforms use the OS
local app configuration directory.

## What’s inside?

- [Global settings](global-config.md) and downloaded archives.
- Dependency repositories in `packages/<host>/<user>/<repo>/<branch>`.
  Sampctl moves an older cached checkout to the host directory only if its
  Git origin matches the requested dependency.
- Installed compilers in `pawn/<user>/<repo>/<version>`, including their
  supporting files.

Run `sampctl check` to inspect cached compilers. `check --fix` can finish recorded
recovery, restore execute permissions, or reinstall from a cached archive.
External compilers and unrecognized files need manual inspection.

## Is it safe to delete?

Stop running builds and finish pending recovery first. Keep global settings
and any backups needed for recovery. Sampctl downloads missing artifacts on
the next ensure, build, or run.

An interrupted extraction can leave an unrecorded temporary folder. Inspect it
before removing it.
