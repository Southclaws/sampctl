# Install sampctl

## Download a release

1. Download the archive for your operating system and CPU from the
   [GitHub releases page](https://github.com/Southclaws/sampctl/releases).
2. Extract it and put `sampctl` somewhere on your `PATH`.

## Windows: add sampctl to PATH (manual install)

1. Put `sampctl.exe` in a folder such as `C:\sampctl\`.
2. Search for **Environment Variables** in the Start menu. Open **Edit the
   system environment variables**, select **Environment Variables**, then edit
   `Path` under **User variables** and add the folder.
3. Open a new PowerShell or Command Prompt window and run `sampctl version`.

## Linux installer scripts

Each script downloads the latest GitHub release for your CPU architecture.

Binary tarball (no package manager):

```bash
curl -fsSL https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-bin.sh | bash
```

Debian/Ubuntu (`.deb` via `dpkg`):

```bash
curl -fsSL https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-deb.sh | bash
```

RPM-based distros (`.rpm` via `rpm`):

```bash
curl -fsSL https://raw.githubusercontent.com/Southclaws/sampctl/master/scripts/install-rpm.sh | bash
```

The Debian installer uses `sudo dpkg -i`.

## Homebrew on Linux

Releases include a [Homebrew cask](../Casks/sampctl.rb) for Linux.

## Scoop on Windows

After [installing Scoop](https://scoop.sh/), add the sampctl repository as a
bucket and install the package:

```powershell
scoop bucket add sampctl https://github.com/Southclaws/sampctl
scoop install sampctl/sampctl
```

For a one-time install, use the manifest URL:

```powershell
scoop install https://raw.githubusercontent.com/Southclaws/sampctl/master/sampctl.json
```

Use the bucket method if you want updates through `scoop update`.

## Verify it works

```bash
sampctl version
sampctl help
```

## Build from source

Use Go 1.26.9 or newer. Run `task install` to install `sampctl` into `GOBIN`,
or into `$(go env GOPATH)/bin` when `GOBIN` is unset.
