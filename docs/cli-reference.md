# `sampctl`

Manage dependencies, build Pawn scripts, and run SA-MP or open.mp servers.

## Commands (16)

### `sampctl init`

Usage: `sampctl init [--preset samp|openmp]`

Create a SA-MP or open.mp project.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: project directory (default: ".")
- `--preset value`: project preset: 'samp' for SA-MP or 'openmp' for open.mp (default: "openmp")

---

### `sampctl ensure`

Usage: `sampctl ensure [dependency]`

Install dependencies declared in `pawn.json`/`pawn.yaml`.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: project directory (default: ".")
- `--update user/repo`: update dynamic dependencies (user/repo or `user/repo:latest`) to the latest tagged release
- `--force --update`: with --update, also update dependencies pinned to explicit tags
- `--no-lock`: resolve dependencies without reading or writing pawn.lock
- `--lock-only`: update pawn.lock without changing installed dependencies

---

### `sampctl install`

Usage: `sampctl install [package definition]`

Add dependencies to `pawn.json`/`pawn.yaml` and install them.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: project directory (default: ".")
- `--dev`: add development or test dependencies

---

### `sampctl uninstall`

Usage: `sampctl uninstall [package definition]`

Remove dependencies from `pawn.json`/`pawn.yaml` and update an existing pawn.lock. Installed files remain.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: project directory (default: ".")
- `--dev`: remove development dependencies

---

### `sampctl release`

Usage: `sampctl release`

Tag the next version and create a GitHub release with archived package files.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: project directory (default: ".")

---

### `sampctl config`

Usage: `sampctl config [field] [value]`

View or change sampctl settings.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration

---

### `sampctl get`

Usage: `sampctl get [package definition] (target path)`

Clone a GitHub package and install its dependencies. Use the current directory if empty, or a directory named after the repository.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration

---

### `sampctl build`

Usage: `sampctl build [build name]`

Build a package defined in `pawn.json`/`pawn.yaml`.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: project directory (default: ".")
- `--forceEnsure`: install dependencies before building
- `--dryRun`: print the build command without running it
- `--watch`: rebuild when source files change
- `--buildFile value`: file for the incrementing build number
- `--relativePaths`: use relative paths in compiler output
- `--no-lock`: disable lockfile support

---

### `sampctl run`

Usage: `sampctl run`

Compile and run a package defined in `pawn.json`/`pawn.yaml`.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: server directory (default: ".")
- `--container`: run the server in a Linux container
- `--build --forceBuild`: build configuration to use if --forceBuild is set
- `--forceBuild`: build before starting the server
- `--forceEnsure --forceBuild`: install dependencies before building when --forceBuild is set
- `--noCache --forceEnsure`: download plugins again when --forceEnsure is set
- `--watch`: rebuild when source files change
- `--buildFile value`: file for the incrementing build number
- `--relativePaths`: use relative paths in compiler output

---

### `sampctl check`

Usage: `sampctl check [--fix]`

Checks your configuration, dependencies, runtime, compiler, and cache. Use --fix to repair supported issues and check the result.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--fix`: repair supported issues and check the result
- `--dir value`: project directory (searches parent directories) (default: ".")

---

### `sampctl compiler`

Usage: `sampctl compiler`

Manage compiler settings.

#### Subcommands (1)

### `sampctl compiler list`

Usage: `sampctl compiler list`

List compiler presets and their settings.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration


---

### `sampctl template`

Usage: `sampctl template <subcommand>`

Create and use package templates.

#### Subcommands (3)

### `sampctl template make`

Usage: `sampctl template make [name]`

Create a package template.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--dir value`: package directory (default: ".")
- `--update`: update cached dependencies to latest version

### `sampctl template build`

Usage: `sampctl template build [template] [filename]`

Build a script using a package template.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration

### `sampctl template run`

Usage: `sampctl template run [template] [filename]`

Build and run a script using a package template.

#### Flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--version value`: SA-MP server version (default: "0.3.7")
- `--mode value`: runtime mode, one of: server, main, y_testing (default: "main")


---

### `sampctl version`

Show the sampctl version, also used for `--container` images.

---

### `sampctl completion`

Install shell completion.

---

### `sampctl docs`

Usage: `sampctl docs > documentation.md`

Print command documentation as Markdown.

---

### `sampctl help`

Usage: `Shows a list of commands or help for one command`

---

## Global flags

- `--verbose`: show detailed logs and error causes
- `--platform value`: target platform for downloaded binaries: windows, linux, or darwin
- `--bare`: skip all pre-run configuration
- `--help, -h`: show help
- `--appVersion, -V, --version`: sampctl version
