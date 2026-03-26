package pkgcontext

import (
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type PackageServices struct {
	GitHub         *github.Client
	GitAuth        transport.AuthMethod
	Platform       string
	CacheDir       string
	RemotePackages pawnpackage.RemotePackageFetcher
	RepoStore      RepositoryStore
	RepoHealth     RepositoryHealth
	RuntimeEnv     RuntimeEnvironment
	RuntimeProv    RuntimeProvisioner
}

func (services PackageServices) repositoryStore() RepositoryStore {
	if services.RepoStore != nil {
		return services.RepoStore
	}
	return GitRepositoryStore{}
}

func (services PackageServices) repositoryHealth() RepositoryHealth {
	if services.RepoHealth != nil {
		return services.RepoHealth
	}
	return GitRepositoryHealth{}
}

func (services PackageServices) runtimeEnvironment() RuntimeEnvironment {
	if services.RuntimeEnv != nil {
		return services.RuntimeEnv
	}
	return runtimeEnvironmentAdapter{}
}

func (services PackageServices) runtimeProvisioner() RuntimeProvisioner {
	if services.RuntimeProv != nil {
		return services.RuntimeProv
	}
	return runtimeProvisionerAdapter{}
}

type PackageResolvedState struct {
	AllDependencies []versioning.DependencyMeta
	AllPlugins      []versioning.DependencyMeta
	AllIncludePaths []string
	ActualRuntime   runtimecfg.Runtime
	ActualBuild     build.Config
}

type PackageExecutionState struct {
	Runtime     string
	Container   bool
	AppVersion  string
	BuildName   string
	ForceBuild  bool
	ForceEnsure bool
	NoCache     bool
	BuildFile   string
	Relative    bool
}

type PackageLockfileState struct {
	lockfileResolver DependencyLock
	UseLockfile      bool
}

func (state *PackageLockfileState) SetLockfileResolver(resolver DependencyLock) {
	if state == nil {
		return
	}
	state.lockfileResolver = resolver
}

func (state *PackageLockfileState) LockedVersion(meta versioning.DependencyMeta, forceUpdate bool) versioning.DependencyMeta {
	if state == nil || state.lockfileResolver == nil || forceUpdate {
		return meta
	}
	return state.lockfileResolver.GetLockedVersion(meta)
}

func (state *PackageLockfileState) PreviousDependency(meta versioning.DependencyMeta) (lockfile.LockedDependency, bool) {
	if state == nil || state.lockfileResolver == nil {
		return lockfile.LockedDependency{}, false
	}
	return state.lockfileResolver.GetPreviousDependency(meta)
}

func (state *PackageLockfileState) RecordDependencyResolution(
	meta versioning.DependencyMeta,
	resolution lockfile.DependencyResolution,
	transitive bool,
	requiredBy string,
) error {
	if state == nil || state.lockfileResolver == nil {
		return nil
	}
	return state.lockfileResolver.RecordResolution(meta, resolution, transitive, requiredBy)
}

func (state *PackageLockfileState) RecordRuntime(
	version string,
	platform string,
	runtimeType string,
	files []lockfile.LockedFileInfo,
) {
	if state == nil || state.lockfileResolver == nil {
		return
	}
	state.lockfileResolver.RecordRuntime(version, platform, runtimeType, files)
}

func (state *PackageLockfileState) RecordBuild(record lockfile.BuildRecord) {
	if state == nil || state.lockfileResolver == nil {
		return
	}
	state.lockfileResolver.RecordBuild(record)
}

func (state *PackageLockfileState) SaveLockfile() error {
	if state == nil || state.lockfileResolver == nil {
		return nil
	}
	return state.lockfileResolver.Save()
}

func (state *PackageLockfileState) HasLockfile() bool {
	return state != nil && state.lockfileResolver != nil && state.lockfileResolver.HasLockfile()
}

func (state *PackageLockfileState) ForceUpdateLockfile() {
	if state == nil || state.lockfileResolver == nil {
		return
	}
	state.lockfileResolver.ForceUpdate()
}

func (state *PackageLockfileState) HasLockfileResolver() bool {
	return state != nil && state.lockfileResolver != nil
}

func (state *PackageLockfileState) GetLockfile() *lockfile.Lockfile {
	if state == nil || state.lockfileResolver == nil {
		return nil
	}
	return state.lockfileResolver.GetLockfile()
}
