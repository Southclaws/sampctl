package pkgcontext

import (
	"context"

	git "github.com/go-git/go-git/v5"
	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// DependencyLock abstracts lockfile-aware dependency resolution for package flows.
type DependencyLock interface {
	GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta
	RecordResolution(meta versioning.DependencyMeta, resolution lockfile.DependencyResolution, transitive bool, requiredBy string) error
	RecordLocalDependency(meta versioning.DependencyMeta) error
	RecordRuntime(version, platform, runtimeType string, files []lockfile.LockedFileInfo)
	RecordBuild(record lockfile.BuildRecord)
	Save() error
	ForceUpdate()
	HasLockfile() bool
	GetLockfile() *lockfile.Lockfile
}

// LockfileInitializer initializes lockfile resolution for a package context.
type LockfileInitializer interface {
	InitLockfileResolver(sampctlVersion string) error
}

// LockfileController exposes the command-facing lockfile behavior supported by PackageContext.
type LockfileController interface {
	SaveLockfile() error
	HasLockfile() bool
	ForceUpdateLockfile()
	HasLockfileResolver() bool
	GetLockfile() *lockfile.Lockfile
}

// BuildLockfileController extends LockfileController with build recording behavior.
type BuildLockfileController interface {
	LockfileController
	RecordBuildToLockfile(compilerVersion, compilerPreset, entry, output string)
}

// RepositoryStore abstracts repository open/clone operations used by package flows.
type RepositoryStore interface {
	Open(path string) (*git.Repository, error)
	Clone(path string, isBare bool, opts *git.CloneOptions) (*git.Repository, error)
}

// RepositoryHealth abstracts repository validation and repair operations.
type RepositoryHealth interface {
	Validate(path string) (bool, error)
	Repair(path string) error
}

// RuntimeEnvironment abstracts runtime preparation and execution for package runs.
type RuntimeEnvironment interface {
	Run(ctx context.Context, cfg runtimecfg.Runtime, options runtimepkg.RunOptions) error
	PrepareRuntimeDirectory(cacheDir, version, platform, scriptfiles string) error
	CopyFileToRuntime(cacheDir, version, amxFile string) error
	Ensure(ctx context.Context, gh *github.Client, cfg *runtimecfg.Runtime, noCache bool) error
	GenerateConfig(cfg *runtimecfg.Runtime) error
}

// RuntimeProvisioner abstracts runtime layout/binary/plugin provisioning for ensure flows.
type RuntimeProvisioner interface {
	EnsurePackageLayout(workingDir string, isOpenMP bool) error
	EnsureBinaries(ctx context.Context, cacheDir string, cfg runtimecfg.Runtime) (*runtimepkg.RuntimeManifestInfo, error)
	EnsurePlugins(request runtimepkg.EnsurePluginsRequest) error
}
