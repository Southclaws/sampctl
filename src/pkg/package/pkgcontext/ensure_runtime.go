package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	if err := validateRuntimeDirectory(pcx.Package.LocalPath, cfg.WorkingDir); err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure layout", Err: err}
	}

	provisioner := pcx.PackageServices.runtimeProvisioner()
	if _, native := provisioner.(runtimeProvisionerAdapter); native && pcx.HasLockfileResolver() && filepath.Clean(cfg.WorkingDir) != filepath.Clean(pcx.Package.LocalPath) {
		if err := regularRecoveryFile(filepath.Join(pcx.Package.LocalPath, lockfile.Filename)); err != nil {
			return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "read previous ownership", Err: err}
		}

		previous, err := lockfile.Load(pcx.Package.LocalPath)
		if err != nil {
			return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "read previous ownership", Err: err}
		}
		if previous != nil {
			if _, err := runtimepkg.AdoptRuntimeOwnership(ctx, cfg.WorkingDir, previous.Runtime); err != nil {
				return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "adopt previous ownership", Err: err}
			}
		}
	}

	if err := provisioner.EnsurePackageLayout(cfg.WorkingDir, cfg.IsOpenMP()); err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure layout", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	runtimeInfo, err := provisioner.EnsureBinaries(ctx, pcx.CacheDir, cfg)
	if err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure binaries", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := provisioner.EnsurePlugins(runtimepkg.EnsurePluginsRequest{
		Context:  ctx,
		GitHub:   pcx.GitHub,
		Config:   &pcx.ActualRuntime,
		CacheDir: pcx.CacheDir,
		NoCache:  false,
	}); err != nil {
		return &DependencyError{Dependency: "runtime " + cfg.Version, Operation: "ensure plugins", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	pcx.recordRuntimeToLockfile(runtimeInfo)
	if pcx.dependencyInstalls == nil {
		if err := pcx.PackageLockfileState.SaveLockfile(); err != nil {
			return &OperationError{Operation: "lockfile", Err: err}
		}
	}

	return nil
}

func validateRuntimeDirectory(project, directory string) error {
	root := filepath.Clean(directory)
	if relative, err := filepath.Rel(project, directory); err == nil && filepath.IsLocal(relative) {
		root = filepath.Clean(project)
	}

	for path := filepath.Clean(directory); ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("runtime directory %s contains a symlink or non-directory: %w", path, os.ErrInvalid)
		}

		if path == root {
			return nil
		}
	}
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
	outputPath := packagePath(pcx.Package.LocalPath, output)
	if output != "" && fs.Exists(outputPath) {
		hash, err := hashOutputFile(outputPath)
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
