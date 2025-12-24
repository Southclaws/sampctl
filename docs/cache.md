# Cache

`sampctl` keeps downloads (dependencies, runtimes, plugins/components, compilers) in a local cache so it doesn’t need to re-download things every time.

## Where is the cache?

On Linux, the cache/config directory is typically:

- `~/.config/sampctl`

On other platforms it uses the OS “local app config” directory for the app name `sampctl`.

## What’s inside?

You may see files and folders like:

- `config.json` / `config.yaml` (global `sampctl` settings)
- downloaded runtime archives and staging files
- downloaded plugin/component resources
- cached compiler downloads

Your project folder may also contain:

- `.sampctl/sampctl-runtime-manifest.json` (tracks which runtime files were installed into that folder)

## Is it safe to delete?

Yes. You can delete the cache directory at any time.

`sampctl` will re-download whatever it needs the next time you run `sampctl ensure`, `sampctl build`, or `sampctl run`.
