# Dependency schemes (when to use what)

Use `user/repo` for most Pawn libraries. Add a scheme such as `plugin://` to
tell sampctl how to use a dependency.

## Regular packages: `user/repo`

Use this for most include libraries.

```json
{
  "dependencies": ["pawn-lang/samp-stdlib", "Southclaws/samp-logger"]
}
```

What happens:

- The repo is cloned into `./dependencies/<repo>/`.
- The dependency folders are added to the compiler include path during builds.

## Plugins: `plugin://user/repo`

Use this when you want a dependency to be treated as a **server plugin binary**.

```json
{
  "dependencies": ["plugin://samp-incognito/samp-streamer-plugin"]
}
```

What happens:

- The plugin package is ensured like a normal repo dependency.
- `sampctl` downloads a matching plugin release asset (based on the dependency’s `resources` metadata).
- The plugin binary is extracted into `./plugins/`.
- The plugin is automatically added to the runtime plugin list.

## Components (open.mp): `component://user/repo`

Use this for open.mp components you want downloaded/installed and loaded.

```json
{
  "dependencies": ["component://katursis/Pawn.RakNet:1.6.0-omp"]
}
```

What happens:

- Works like `plugin://...` but installs to `./components/` and adds to the runtime component list.

## Filterscripts: `filterscript://user/repo`

Use this to fetch a filterscript's source repository.

```json
{
  "dependencies": ["filterscript://SomeUser/some-filterscript"]
}
```

Sampctl installs the repository under `./dependencies/<repo>/`. To use the
script at runtime, build it and place the resulting `.amx` in `filterscripts/`.

## Extra include paths: `includes://user/repo`

Use this for repos that should only contribute include paths (or for “monorepo” layouts).

```json
{
  "dependencies": ["includes://SomeUser/some-includes-repo"]
}
```

What happens:

- `sampctl` ensures the repo and adds its include paths for compilation.

## Version pinning works with schemes

Schemes support pinning the same way as regular dependencies:

- Tag: `plugin://user/repo:1.2.3`
- Branch: `plugin://user/repo@branch-name`
- Commit: `plugin://user/repo#<sha1>`

See also:

- [Dependencies](dependencies.md)
- [Plugin resources (for plugin authors)](plugin-resources.md)
