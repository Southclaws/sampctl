package pkgcontext

import (
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
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
	Package pawnpackage.Package // the package this context wraps

	PackageServices
	PackageResolvedState
	PackageExecutionState
	PackageLockfileState
}

type NewPackageContextOptions struct {
	GitHub         *github.Client
	Auth           transport.AuthMethod
	Parent         bool
	Dir            string
	Platform       string
	CacheDir       string
	Vendor         string
	Init           bool
	UseLockfile    bool
	AppVersion     string
	RemotePackages pawnpackage.RemotePackageFetcher
	RepoStore      RepositoryStore
	RepoHealth     RepositoryHealth
	RuntimeEnv     RuntimeEnvironment
}

// NewPackageContext attempts to parse a directory as a Package by looking for a
// `pawn.json` or `pawn.yaml` file and unmarshalling it - additional parameters
// are required to specify whether or not the package is a "parent package" and
// where the vendor directory is.
func NewPackageContext(options NewPackageContextOptions) (pcx *PackageContext, err error) {
	pcx = newPackageContextBase(options)
	if err = pcx.loadPackageDefinition(options); err != nil {
		return nil, err
	}
	if err = pcx.ensureCachedDependencies(); err != nil {
		return nil, err
	}
	print.Verb(pcx.Package, "flattened dependencies to", len(pcx.AllDependencies), "leaves")
	return pcx, nil
}

func newPackageContextBase(options NewPackageContextOptions) *PackageContext {
	remotePackages := options.RemotePackages
	if remotePackages == nil {
		remotePackages = pawnpackage.NewRemotePackageFetcher(options.GitHub)
	}

	repoStore := options.RepoStore
	if repoStore == nil {
		repoStore = GitRepositoryStore{}
	}

	repoHealth := options.RepoHealth
	if repoHealth == nil {
		repoHealth = GitRepositoryHealth{}
	}

	runtimeEnv := options.RuntimeEnv
	if runtimeEnv == nil {
		runtimeEnv = runtimeEnvironmentAdapter{}
	}

	return &PackageContext{
		PackageServices: PackageServices{
			GitHub:         options.GitHub,
			GitAuth:        options.Auth,
			Platform:       options.Platform,
			CacheDir:       options.CacheDir,
			RemotePackages: remotePackages,
			RepoStore:      repoStore,
			RepoHealth:     repoHealth,
			RuntimeEnv:     runtimeEnv,
		},
	}
}

func (pcx *PackageContext) loadPackageDefinition(options NewPackageContextOptions) (err error) {
	pcx.Package, err = pawnpackage.PackageFromDir(options.Dir)
	if err != nil {
		err = errors.Wrap(err, "failed to read package definition")
		return
	}

	pcx.Package.Parent = options.Parent
	pcx.Package.LocalPath = options.Dir

	if !options.Init {
		pcx.Package.Tag = getPackageTag(options.Dir)
	} else {
		pcx.Package.Tag = ""
	}

	print.Verb(pcx.Package, "read package from directory", options.Dir)

	if options.Vendor == "" {
		pcx.Package.Vendor = filepath.Join(options.Dir, "dependencies")
	} else {
		pcx.Package.Vendor = options.Vendor
	}

	if err = pcx.Package.Validate(); err != nil {
		err = errors.Wrap(err, "package validation failed during initial read")
		return
	}

	// user and repo are not mandatory but are recommended, warn the user if this is their own
	// package (parent == true) but ignore for dependencies (parent == false)
	if !options.Init {
		if pcx.Package.User == "" {
			if options.Parent {
				print.Warn(pcx.Package, "Package Definition File does specify a value for `user`.")
			}
			pcx.Package.User = "<none>"
		}
		if pcx.Package.Repo == "" {
			if options.Parent {
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

	return nil
}

func (pcx *PackageContext) ensureCachedDependencies() error {
	print.Verb(pcx.Package, "building dependency tree and ensuring cached copies")
	if err := pcx.EnsureDependenciesCached(); err != nil {
		return errors.Wrap(err, "failed to ensure dependencies are cached")
	}
	return nil
}

// NewPackageContextWithLockfile creates a PackageContext with lockfile support enabled.
func NewPackageContextWithLockfile(options NewPackageContextOptions) (pcx *PackageContext, err error) {
	pcx, err = NewPackageContext(options)
	if err != nil {
		return nil, err
	}

	pcx.UseLockfile = options.UseLockfile
	pcx.AppVersion = options.AppVersion

	if options.UseLockfile && options.Parent {
		resolver, resolverErr := newDependencyLock(options.Dir, options.AppVersion)
		if resolverErr != nil {
			return nil, errors.Wrap(resolverErr, "failed to initialize lockfile resolver")
		}
		pcx.PackageLockfileState.SetLockfileResolver(resolver)
	}

	return pcx, nil
}

// InitLockfileResolver initializes the lockfile resolver for an existing PackageContext.
// This can be called after NewPackageContext if lockfile support wasn't enabled initially.
func (pcx *PackageContext) InitLockfileResolver(sampctlVersion string) error {
	if !pcx.Package.Parent {
		return nil // Only parent packages use lockfiles
	}

	resolver, err := newDependencyLock(pcx.Package.LocalPath, sampctlVersion)
	if err != nil {
		return errors.Wrap(err, "failed to initialize lockfile resolver")
	}
	pcx.PackageLockfileState.SetLockfileResolver(resolver)

	pcx.UseLockfile = true
	pcx.AppVersion = sampctlVersion
	return nil
}

// SaveLockfile saves the lockfile if it was modified during dependency resolution
func (pcx *PackageContext) SaveLockfile() error {
	return pcx.PackageLockfileState.SaveLockfile()
}

// HasLockfile returns true if the package has a lockfile
func (pcx *PackageContext) HasLockfile() bool {
	return pcx.PackageLockfileState.HasLockfile()
}

// ForceUpdateLockfile resets the lockfile state so fresh versions are resolved.
func (pcx *PackageContext) ForceUpdateLockfile() {
	pcx.PackageLockfileState.ForceUpdateLockfile()
}

// HasLockfileResolver reports whether lockfile support is enabled for the package context.
func (pcx *PackageContext) HasLockfileResolver() bool {
	return pcx.PackageLockfileState.HasLockfileResolver()
}

// GetLockfile returns the current in-memory lockfile, if one is active.
func (pcx *PackageContext) GetLockfile() *lockfile.Lockfile {
	return pcx.PackageLockfileState.GetLockfile()
}

func newDependencyLock(dir, sampctlVersion string) (DependencyLock, error) {
	return lockfile.NewResolver(dir, sampctlVersion, true)
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
			print.Info("Package does not have any tags, consider versioning your code with: `sampctl release`")
		} else if vtag != nil {
			tag = vtag.Name
		}
	}
	return
}
