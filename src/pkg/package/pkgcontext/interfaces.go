package pkgcontext

import (
	git "github.com/go-git/go-git/v5"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

// DependencyLock abstracts lockfile-aware dependency resolution for package flows.
type DependencyLock interface {
	GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta
	RecordResolution(meta versioning.DependencyMeta, repo *git.Repository, transitive bool, requiredBy string) error
	RecordLocalDependency(meta versioning.DependencyMeta) error
	RecordRuntime(version, platform, runtimeType string, files []lockfile.LockedFileInfo)
	RecordBuild(compilerVersion, compilerPreset, entry, output, outputHash string)
	Save() error
	ForceUpdate()
	HasLockfile() bool
	GetLockfile() *lockfile.Lockfile
}

// RepositoryStore abstracts repository open/clone operations used by package flows.
type RepositoryStore interface {
	Open(path string) (*git.Repository, error)
	Clone(path string, isBare bool, opts *git.CloneOptions) (*git.Repository, error)
}
