package pkgcontext

import (
	"context"
	"io"

	git "github.com/go-git/go-git/v5"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

// DependencyLock abstracts lockfile-aware dependency resolution for package flows.
type DependencyLock interface {
	GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta
	RecordResolution(meta versioning.DependencyMeta, resolution lockfile.DependencyResolution, transitive bool, requiredBy string) error
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

// RepositoryHealth abstracts repository validation and repair operations.
type RepositoryHealth interface {
	Validate(path string) (bool, error)
	Repair(path string) error
}

// RuntimeEnvironment abstracts runtime preparation and execution for package runs.
type RuntimeEnvironment interface {
	Run(ctx context.Context, cfg runtimecfg.Runtime, cacheDir string, passArgs, recover bool, output io.Writer, input io.Reader) error
	PrepareRuntimeDirectory(cacheDir, version, platform, scriptfiles string) error
	CopyFileToRuntime(cacheDir, version, amxFile string) error
	Ensure(ctx context.Context, gh any, cfg *runtimecfg.Runtime, noCache bool) error
	GenerateConfig(cfg *runtimecfg.Runtime) error
}
