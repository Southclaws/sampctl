package rook

import (
	"path/filepath"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// PackageContext stores state for a package during its lifecycle.
type PackageContext struct {
	Package         types.Package               // the package this context wraps
	GitHub          *github.Client              // GitHub client for downloading plugins
	GitAuth         transport.AuthMethod        // Authentication method for git
	Platform        string                      // the platform this package targets
	CacheDir        string                      // the cache directory
	AllDependencies []versioning.DependencyMeta // flattened list of dependencies
	AllPlugins      []versioning.DependencyMeta // flattened list of plugin dependencies
	AllIncludePaths []string                    // any additional include paths specified by resources
	ActualBuild     types.BuildConfig           // actual build configuration to use for running the package
	ActualRuntime   types.Runtime               // actual runtime configuration to use for running the package

	// Runtime specific fields
	Runtime     string // the runtime config to use, defaults to `default`
	Container   bool   // whether or not to run the package in a container
	AppVersion  string // the version of sampctl
	BuildName   string // Build configuration to use
	ForceBuild  bool   // Force a build before running
	ForceEnsure bool   // Force an ensure before building before running
	NoCache     bool   // Don't use a cache, download all plugin dependencies
	BuildFile   string // File to increment build number
	Relative    bool   // Show output as relative paths

}

// NewPackageContext attempts to parse a directory as a Package by looking for a
// `pawn.json` or `pawn.yaml` file and unmarshalling it - additional parameters
// are required to specify whether or not the package is a "parent package" and
// where the vendor directory is.
func NewPackageContext(
	gh *github.Client,
	auth transport.AuthMethod,
	parent bool,
	dir string,
	platform string,
	cacheDir string,
	vendor string,
) (pcx *PackageContext, err error) {
	pcx = &PackageContext{
		GitHub:   gh,
		GitAuth:  auth,
		Platform: platform,
		CacheDir: cacheDir,
	}
	pcx.Package, err = types.PackageFromDir(dir)
	if err != nil {
		err = errors.Wrap(err, "failed to read package definition")
		return
	}

	pcx.Package.Parent = parent
	pcx.Package.LocalPath = dir
	pcx.Package.Tag = getPackageTag(dir)

	print.Verb(pcx.Package, "read package from directory", dir)

	if vendor == "" {
		pcx.Package.Vendor = filepath.Join(dir, "dependencies")
	} else {
		pcx.Package.Vendor = vendor
	}

	if err = pcx.Package.Validate(); err != nil {
		err = errors.Wrap(err, "package validation failed during initial read")
		return
	}

	// user and repo are not mandatory but are recommended, warn the user if this is their own
	// package (parent == true) but ignore for dependencies (parent == false)
	if pcx.Package.User == "" {
		if parent {
			print.Warn(pcx.Package, "Package Definition File does specify a value for `user`.")
		}
		pcx.Package.User = "<none>"
	}
	if pcx.Package.Repo == "" {
		if parent {
			print.Warn(pcx.Package, "Package Definition File does specify a value for `repo`.")
		}
		pcx.Package.Repo = "<local>"
	}

	// if there is no runtime configuration, use the defaults
	if pcx.Package.Runtime == nil {
		pcx.Package.Runtime = new(types.Runtime)
	}
	types.ApplyRuntimeDefaults(pcx.Package.Runtime)

	print.Verb(pcx.Package, "building dependency tree and ensuring cached copies")
	err = pcx.EnsureDependenciesCached()
	if err != nil {
		err = errors.Wrap(err, "failed to ensure dependencies are cached")
		return
	}

	print.Verb(pcx.Package, "flattened dependencies to", len(pcx.AllDependencies), "leaves")
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
