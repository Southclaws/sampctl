package pkgcontext

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
)

func (pcx *PackageContext) ensureParentRuntime(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg, err := pcx.Package.GetRuntimeConfig(pcx.Runtime)
	if err != nil {
		return &DependencyError{Dependency: "runtime", Operation: "read configuration", Err: err}
	}

	cfg.WorkingDir = pcx.Package.RuntimeWorkingDir()
	cfg.Platform = pcx.Platform
	cfg.Format = pcx.Package.Format

	cfg.PluginDeps, err = pcx.GatherPlugins()
	if err != nil {
		return err
	}

	pcx.ActualRuntime = cfg

	if err := pcx.PackageServices.runtimeProvisioner().EnsurePackageLayout(cfg.WorkingDir, cfg.IsOpenMP()); err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure layout", Err: err}
	}

	runtimeInfo, err := pcx.PackageServices.runtimeProvisioner().EnsureBinaries(ctx, pcx.CacheDir, cfg)
	if err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure binaries", Err: err}
	}

	if err := pcx.PackageServices.runtimeProvisioner().EnsurePlugins(runtimepkg.EnsurePluginsRequest{
		Context:  ctx,
		GitHub:   pcx.GitHub,
		Config:   &pcx.ActualRuntime,
		CacheDir: pcx.CacheDir,
		NoCache:  false,
	}); err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure plugins", Err: err}
	}

	pcx.recordRuntimeToLockfile(runtimeInfo)
	if pcx.dependencyInstalls == nil {
		if err := pcx.PackageLockfileState.SaveLockfile(); err != nil {
			return &OperationError{Operation: "lockfile", Err: err}
		}
	}

	return nil
}

func (pcx *PackageContext) recordRuntimeToLockfile(manifestInfo *runtimepkg.RuntimeManifestInfo) {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return
	}

	if manifestInfo == nil {
		print.Verb("no runtime info available, skipping lockfile runtime record")
		return
	}

	files := make([]lockfile.LockedFileInfo, len(manifestInfo.Files))
	for i, fileInfo := range manifestInfo.Files {
		files[i] = lockfile.LockedFileInfo{
			Path: fileInfo.Path,
			Size: fileInfo.Size,
			Hash: fileInfo.Hash,
			Mode: fileInfo.Mode,
		}
	}

	print.Verb("recording runtime to lockfile:", pcx.ActualRuntime.Version, pcx.ActualRuntime.Platform, string(pcx.ActualRuntime.RuntimeType))
	pcx.PackageLockfileState.RecordRuntime(
		pcx.ActualRuntime.Version,
		pcx.ActualRuntime.Platform,
		string(pcx.ActualRuntime.RuntimeType),
		files,
	)
}

func (pcx *PackageContext) RecordBuildToLockfile(compilerVersion, compilerPreset, entry, output string) {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return
	}

	outputHash := ""
	if output != "" && fs.Exists(output) {
		hash, err := hashOutputFile(output)
		if err != nil {
			print.Warn("failed to hash output file:", err)
		} else {
			outputHash = hash
		}
	}

	pcx.PackageLockfileState.RecordBuild(lockfile.BuildRecord{
		CompilerVersion: compilerVersion,
		CompilerPreset:  compilerPreset,
		Entry:           entry,
		Output:          output,
		OutputHash:      outputHash,
	})
}
