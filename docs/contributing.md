# Contributing to sampctl

Thank you for your interest in contributing to `sampctl`. Contributions are welcome in the form of bug fixes, features, documentation improvements, tests, and issue reports.

This guide describes the development workflow used by the repository. It is intentionally focused on the checks that can be run locally before opening a pull request.

## Before you start

The project currently requires **Go 1.25.0**, as declared in `go.mod`. You also need Git. The `task` command is useful because the repository's `Taskfile.yml` provides shortcuts for formatting, tests, linting, builds, and CI-related checks.

Earthly is optional for the fastest local development loop, but it is recommended when you want to reproduce the containerized checks used by GitHub Actions. Earthly requires a compatible container runtime according to your local setup.

## Clone the repository

Fork the repository on GitHub, then clone your fork and configure the upstream repository:

```bash
git clone git@github.com:<your-user>/sampctl.git
cd sampctl
git remote add upstream https://github.com/Southclaws/sampctl.git
git fetch upstream
```

Create a focused branch from the current upstream default branch:

```bash
git switch master
git pull --ff-only upstream master
git switch -c feat/short-description
```

Use a branch name that describes the change, such as `fix/compiler-archive-files` or `docs/contributing-guide`.

## Repository layout

The main Go entrypoint is under `src/`. Commands are implemented in `src/commands`, while the main package, runtime, build, dependency, filesystem, and resource logic lives under `src/pkg`.

| Path | Purpose |
|---|---|
| `src/` | Go application and command-line entrypoint. |
| `src/commands/` | CLI command definitions and command state. |
| `src/pkg/` | Build, runtime, package, dependency, filesystem, and resource logic. |
| `docs/` | User and contributor documentation. |
| `.github/workflows/` | GitHub Actions workflows for quality checks, tests, builds, and releases. |
| `Earthfile` | Containerized quality, test, and build targets. |
| `Taskfile.yml` | Local shortcuts for common development tasks. |

## Set up a local development loop

Download and verify the Go dependencies:

```bash
go mod download
go mod verify
```

Build a fast local binary with:

```bash
task fast
```

The binary is written to `./sampctl`. You can also build it directly without Task:

```bash
go build -ldflags "-X main.version=dev" -o sampctl ./src
```

Run commands directly from the source tree while iterating:

```bash
go run ./src help
go run ./src version
```

Do not commit locally generated binaries, archives, compiler caches, dependency caches, or temporary runtime directories.

## Format and validate Go code

Format modified Go files before committing:

```bash
task fmt
```

Run the focused checks during development:

```bash
task vet
task lint
task test
```

The complete local Go quality suite is:

```bash
task check
```

`task check` runs linting, `go vet`, static analysis, vulnerability checks, and the race-enabled test suite. Some checks download tools or dependencies and may require network access.

If Task is not installed, the core equivalents are:

```bash
gofmt -w ./src
go vet ./...
go test -race -timeout=10m ./...
```

## Reproduce the containerized CI checks

The repository defines Earthly targets corresponding to the main CI stages:

```bash
task earthly:quality
task earthly:test
task earthly:build-all
```

For the complete quality suite, including static analysis and vulnerability scanning, use:

```bash
task earthly:quality-full
```

GitHub Actions runs the `Build` workflow for pushes and pull requests. It performs Go quality checks, runs the race-enabled tests, builds Linux and Windows binaries, and uploads the resulting binaries as workflow artifacts. The workflow may require approval when it runs for a pull request from a fork; this is a repository permission step, not a code failure.

## Writing tests

Add regression tests close to the package that owns the behavior being changed. Prefer deterministic tests that use temporary directories and local fixtures instead of network services. When a change affects both SA-MP and open.mp, cover the common behavior and add runtime-specific cases where their configuration or layout differs.

For changes involving files, archives, caches, runtimes, or subprocesses, test both the successful path and the relevant failure path. Verify that temporary files and directories are cleaned up, and make sure errors from file operations are handled rather than ignored.

Before opening a pull request, run at least:

```bash
task fmt
task vet
task test
git diff --check
```

## Writing documentation

Documentation belongs under `docs/` and should be linked from `docs/index.md` when it represents a new guide. Keep examples consistent with the current command names and configuration schema. When documenting a new option, explain its default behavior and avoid implying that optional settings are required.

## Commits and pull requests

Keep each pull request focused on one problem or closely related group of changes. Include tests and documentation when they are relevant to the behavior being changed. Describe the problem, the chosen solution, compatibility considerations, and the validation performed.

A typical workflow is:

```bash
git status
git add <changed-files>
git commit -m "fix: describe the behavior being corrected"
git push -u origin feat/short-description
```

Open the pull request from your fork to `Southclaws/sampctl` and the `master` branch. If the pull request addresses an existing issue, reference it in the description. Use `Fixes #123` when the issue should close automatically after merge, or `Related to #123` when the issue should remain open.

After review, push additional commits to the same branch. The pull request will update automatically. Do not force-push a shared branch unless history rewriting has been explicitly agreed with the reviewers.

## Security and credentials

Never commit GitHub tokens, private keys, passwords, compiler archives, or generated runtime artifacts. Tests that require GitHub access should use the repository's existing secret-handling mechanism. If you discover a security issue, do not disclose sensitive details in a public issue; contact the maintainers through an appropriate private channel first.

## Pull request checklist

Before requesting review, confirm that:

- the branch is based on the current `master` branch;
- the change is limited to the stated problem;
- modified Go code is formatted;
- focused tests and the relevant broader tests pass;
- `go vet` and lint checks pass;
- documentation and examples match the implementation;
- no generated binaries, archives, caches, secrets, or temporary runtime files are included;
- the pull request description explains the behavior before and after the change.

Thank you for helping improve `sampctl`.
