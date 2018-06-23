package rook

import (
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// PackageFromDir attempts to parse a directory as a Package by looking for a
// `pawn.json` or `pawn.yaml` file and unmarshalling it - additional parameters
// are required to specify whether or not the package is a "parent package" and
// where the vendor directory is.
func PackageFromDir(parent bool, dir, platform, vendor string) (pkg types.Package, err error) {
	pkg, err = types.PackageFromDir(dir)
	if err != nil {
		err = errors.Wrap(err, "failed to read package definition")
		return
	}

	pkg.Parent = parent
	pkg.LocalPath = dir
	pkg.Tag = getPackageTag(dir)

	print.Verb(pkg, "read package from directory", dir)

	if vendor == "" {
		pkg.Vendor = filepath.Join(dir, "dependencies")
	} else {
		pkg.Vendor = vendor
	}

	if err = pkg.Validate(); err != nil {
		err = errors.Wrap(err, "package validation failed during initial read")
		return
	}

	// user and repo are not mandatory but are recommended, warn the user if this is their own
	// package (parent == true) but ignore for dependencies (parent == false)
	if pkg.User == "" {
		if parent {
			print.Warn("Package Definition File does specify a value for `user`.")
		}
		pkg.User = "<none>"
	}
	if pkg.Repo == "" {
		if parent {
			print.Warn("Package Definition File does specify a value for `repo`.")
		}
		pkg.Repo = "<local>"
	}

	// if there is no runtime configuration, use the defaults
	if pkg.Runtime == nil {
		pkg.Runtime = new(types.Runtime)
	}
	types.ApplyRuntimeDefaults(pkg.Runtime)

	// if this is the user's package (parent == true) and it has dependencies
	// specified but the all-dependencies list is not populated, perform a first
	// run pre-flight cache update for all dependencies.
	if parent && len(pkg.Dependencies) > 0 && len(pkg.AllDependencies) == 0 {
		print.Verb(pkg, "resolving dependencies during package load")
		err = EnsureDependenciesCached(&pkg, platform)
		if err != nil {
			print.Verb("failed to resolve dependency tree:", err)
			err = nil // not a breaking error for PackageFromDir
		}
	}

	return
}

func getPackageTag(dir string) (tag string) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		// repo may be intentionally not a git repo, so only print verbosely
		print.Verb("failed to open repo as git repository:", err)
		err = nil
	} else {
		vtag, errInner := versioning.GetRepoCurrentVersionedTag(repo)
		if errInner != nil {
			// error information only needs to be printed wth --verbose
			print.Verb("failed to get version information:", errInner)
			// but we can let the user know that they should version their code!
			print.Info("Package does not have any tags, consider versioning your code with: `sampctl package release`")
		} else if vtag != nil {
			tag = vtag.Name
		}
	}
	return
}
