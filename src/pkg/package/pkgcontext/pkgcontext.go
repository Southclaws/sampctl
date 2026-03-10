package pkgcontext

import (
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

var _ DependencyLock = (*lockfile.Resolver)(nil)

// GitRepositoryStore is the default repository store backed by go-git.
type GitRepositoryStore struct{}

func (GitRepositoryStore) Open(path string) (*git.Repository, error) {
	return git.PlainOpen(path)
}

func (GitRepositoryStore) Clone(path string, isBare bool, opts *git.CloneOptions) (*git.Repository, error) {
	return git.PlainClone(path, isBare, opts)
}

// GitRepositoryHealth is the default repository health checker backed by package helpers.
type GitRepositoryHealth struct{}

func (GitRepositoryHealth) Validate(path string) (bool, error) {
	return ValidateRepository(path)
}

func (GitRepositoryHealth) Repair(path string) error {
	return RepairRepository(path)
}

// PackageContext stores state for a package during its lifecycle.
type PackageContext struct {
	Package         pawnpackage.Package         // the package this context wraps
	GitHub          *github.Client              // GitHub client for downloading plugins
	GitAuth         transport.AuthMethod        // Authentication method for git
	Platform        string                      // the platform this package targets
	CacheDir        string                      // the cache directory
	AllDependencies []versioning.DependencyMeta // flattened list of dependencies
	AllPlugins      []versioning.DependencyMeta // flattened list of plugin dependencies
	AllIncludePaths []string                    // any additional include paths specified by resources
	ActualRuntime   run.Runtime                 // actual runtime configuration to use for running the package
	ActualBuild     build.Config                // actual build configuration to use for running the package
	RemotePackages  pawnpackage.RemotePackageFetcher
	RepoStore       RepositoryStore
	RepoHealth      RepositoryHealth

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

	// Lockfile support
	lockfileResolver DependencyLock // resolver for lockfile-aware dependency resolution
	UseLockfile      bool           // whether to use lockfile for reproducible builds
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
	init bool,
) (pcx *PackageContext, err error) {
	pcx = &PackageContext{
		GitHub:         gh,
		GitAuth:        auth,
		Platform:       platform,
		CacheDir:       cacheDir,
		RemotePackages: pawnpackage.NewRemotePackageFetcher(gh),
		RepoStore:      GitRepositoryStore{},
		RepoHealth:     GitRepositoryHealth{},
	}
	pcx.Package, err = pawnpackage.PackageFromDir(dir)
	if err != nil {
		err = errors.Wrap(err, "failed to read package definition")
		return
	}

	pcx.Package.Parent = parent
	pcx.Package.LocalPath = dir

	if !init {
		pcx.Package.Tag = getPackageTag(dir)
	} else {
		pcx.Package.Tag = ""
	}

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
	if !init {
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
	}

	// if there is no runtime configuration, initialize an empty one
	// Note: We don't apply defaults here because they would be persisted when
	// WriteDefinition() is called. Defaults are only applied to ActualRuntime
	// during execution
	if pcx.Package.Runtime == nil {
		pcx.Package.Runtime = new(run.Runtime)
	}

	print.Verb(pcx.Package, "building dependency tree and ensuring cached copies")
	err = pcx.EnsureDependenciesCached()
	if err != nil {
		err = errors.Wrap(err, "failed to ensure dependencies are cached")
		return
	}

	print.Verb(pcx.Package, "flattened dependencies to", len(pcx.AllDependencies), "leaves")
	return pcx, nil
}

// NewPackageContextWithLockfile creates a PackageContext with lockfile support enabled.
func NewPackageContextWithLockfile(
	gh *github.Client,
	auth transport.AuthMethod,
	parent bool,
	dir string,
	platform string,
	cacheDir string,
	vendor string,
	init bool,
	useLockfile bool,
	sampctlVersion string,
) (pcx *PackageContext, err error) {
	// Create the base context first
	pcx, err = NewPackageContext(gh, auth, parent, dir, platform, cacheDir, vendor, init)
	if err != nil {
		return nil, err
	}

	// Enable lockfile support if requested
	pcx.UseLockfile = useLockfile
	pcx.AppVersion = sampctlVersion

	if useLockfile && parent {
		pcx.lockfileResolver, err = newDependencyLock(dir, sampctlVersion)
		if err != nil {
			return nil, errors.Wrap(err, "failed to initialize lockfile resolver")
		}
	}

	return pcx, nil
}

// InitLockfileResolver initializes the lockfile resolver for an existing PackageContext.
// This can be called after NewPackageContext if lockfile support wasn't enabled initially.
func (pcx *PackageContext) InitLockfileResolver(sampctlVersion string) error {
	if !pcx.Package.Parent {
		return nil // Only parent packages use lockfiles
	}

	var err error
	pcx.lockfileResolver, err = newDependencyLock(pcx.Package.LocalPath, sampctlVersion)
	if err != nil {
		return errors.Wrap(err, "failed to initialize lockfile resolver")
	}

	pcx.UseLockfile = true
	pcx.AppVersion = sampctlVersion
	return nil
}

// SaveLockfile saves the lockfile if it was modified during dependency resolution
func (pcx *PackageContext) SaveLockfile() error {
	if pcx.lockfileResolver == nil {
		return nil
	}
	return pcx.lockfileResolver.Save()
}

// HasLockfile returns true if the package has a lockfile
func (pcx *PackageContext) HasLockfile() bool {
	return pcx.lockfileResolver != nil && pcx.lockfileResolver.HasLockfile()
}

// ForceUpdateLockfile resets the lockfile state so fresh versions are resolved.
func (pcx *PackageContext) ForceUpdateLockfile() {
	if pcx.lockfileResolver == nil {
		return
	}
	pcx.lockfileResolver.ForceUpdate()
}

// HasLockfileResolver reports whether lockfile support is enabled for the package context.
func (pcx *PackageContext) HasLockfileResolver() bool {
	return pcx.lockfileResolver != nil
}

// GetLockfile returns the current in-memory lockfile, if one is active.
func (pcx *PackageContext) GetLockfile() *lockfile.Lockfile {
	if pcx.lockfileResolver == nil {
		return nil
	}
	return pcx.lockfileResolver.GetLockfile()
}

func newDependencyLock(dir, sampctlVersion string) (DependencyLock, error) {
	return lockfile.NewResolver(dir, sampctlVersion, true)
}

func (pcx PackageContext) repositoryStore() RepositoryStore {
	if pcx.RepoStore != nil {
		return pcx.RepoStore
	}
	return GitRepositoryStore{}
}

func (pcx PackageContext) repositoryHealth() RepositoryHealth {
	if pcx.RepoHealth != nil {
		return pcx.RepoHealth
	}
	return GitRepositoryHealth{}
}

func getPackageTag(dir string) (tag string) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		// repo may be intentionally not a git repo, so only print verbosely
		if errors.Is(err, git.ErrRepositoryNotExists) {
			print.Verb("failed to open repo:", dir, "is not a git repo")
			return
		}
		print.Erro("failed to open repo (", dir, "):", err)
		return
	} else {
		vtag, errInner := versioning.GetRepoCurrentVersionedTag(repo)
		if errInner != nil {
			// error information only needs to be printed with --verbose
			print.Verb("failed to get version information:", errInner)
			// but we can let the user know that they should version their code!
			print.Info("Package does not have any tags, consider versioning your code with: `sampctl package release`")
		} else if vtag != nil {
			tag = vtag.Name
		}
	}
	return
}
