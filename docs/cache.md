# Cache

Sampctl caches dependency, runtime, plugin, and compiler downloads.

## Where is the cache?

On Linux, the cache is usually `~/.config/sampctl`. Other platforms use the OS
local app configuration directory.

## What’s inside?

The cache holds global settings, downloaded archives, and managed installations.
Dependency repositories live under `packages/<host>/<user>/<repo>/<branch>`.
Older repositories move into the host directory only when their configured origin
matches the requested dependency; other legacy copies are preserved.

Compiler versions live under `pawn/<user>/<repo>/<version>`. Installations are
staged and verified before replacement, including supporting files. Builds and
repairs share a lock for that compiler version.

Run `sampctl check` to inspect cached compilers. `check --fix` can finish recorded
recovery, restore execute permissions, or reinstall from a cached archive.
External compilers and unrecognized files need manual inspection.

## Is it safe to delete?

Stop running builds and finish pending recovery first. Keep global settings
and any backups needed for recovery. Sampctl downloads missing artifacts on
the next ensure, build, or run.

An interrupted extraction can leave an unrecorded temporary folder. Inspect it
before removing it.
