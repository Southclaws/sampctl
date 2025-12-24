# Install sampctl

sampctl is a single command-line program named `sampctl` (or `sampctl.exe` on Windows).

## Option A: download a release from GitHub

1. Open the releases page.
2. Download the archive for your OS/CPU.
3. Put `sampctl` somewhere on your `PATH`.

Releases: https://github.com/Southclaws/sampctl/releases

## Windows: add sampctl to PATH (manual install)

If you installed manually (by downloading a release):

1. Put `sampctl.exe` in a folder such as `C:\sampctl\`.
2. Add that folder to your `Path` environment variable.
	- Windows 10/11: Start Menu → search “Environment Variables” → “Edit the system environment variables” → “Environment Variables…” → select `Path` (User variables) → “Edit…” → “New”.
3. Close and re-open PowerShell/Command Prompt, then run `sampctl version`.

## Option B: Linux installer scripts (Deb/RPM/tar)

This repo includes simple installer scripts under `scripts/`.

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

Notes:

- These scripts download the latest GitHub release for your CPU architecture.
- The `.deb` install uses `sudo dpkg -i ...`.
- If you prefer, download the release asset manually instead.

## Option C: Homebrew (Linux)

A Homebrew formula is provided in `Casks/sampctl.rb`. If you use Homebrew, you can also install from a tap/bottle if one is available for your setup.

If you’re not sure, use “Option A” (GitHub releases).

## Option D: Scoop (Windows)

If you use Scoop on Windows, you can install sampctl using the Scoop manifest in this repo (`sampctl.json`).

1. Install Scoop (if you don’t already have it): https://scoop.sh/
2. Add the sampctl bucket:

```powershell
scoop bucket add sampctl https://github.com/Southclaws/sampctl
```

3. Install sampctl:

```powershell
scoop install sampctl
```

**Note**: sampctl is not yet available in the official Scoop buckets (like `main` or `extras`). The above method adds this repository as a custom bucket for easier updates. If you prefer a one-time install without adding a bucket, you can use:

```powershell
scoop install https://raw.githubusercontent.com/Southclaws/sampctl/master/sampctl.json
```

However, this method won’t support automatic updates via `scoop update`.

## Verify it works

```bash
sampctl version
sampctl help
```
