package pkgcontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/pkg/errors"
	"gopkg.in/eapache/go-resiliency.v1/retrier"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/runtime"
	"github.com/Southclaws/sampctl/src/resource"
)

// ErrNotRemotePackage describes a repository that does not contain a package definition file
var ErrNotRemotePackage = errors.New("remote repository does not declare a package")

// EnsureDependencies traverses package dependencies and ensures they are up to date
func (pcx *PackageContext) EnsureDependencies(ctx context.Context, forceUpdate bool) (err error) {
	if pcx.Package.LocalPath == "" {
		return errors.New("package does not represent a locally stored package")
	}

	if !fs.Exists(pcx.Package.LocalPath) {
		return errors.New("package local path does not exist")
	}

	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")

	for _, dependency := range pcx.AllDependencies {
		dep := dependency
		r := retrier.New(retrier.ConstantBackoff(1, 100*time.Millisecond), nil)
		err := r.Run(func() error {
			print.Verb("attempting to ensure dependency", dep)
			errInner := pcx.EnsurePackage(dep, forceUpdate)
			if errInner != nil {
				print.Warn(errors.Wrapf(errInner, "failed to ensure package %s", dep))
				return errInner
			}
			print.Info(pcx.Package, "successfully ensured dependency files for", dep)
			return nil
		})
		if err != nil {
			print.Warn("failed to ensure package", dep, "after 2 attempts, skipping")
			continue
		}
	}

	// Ensure runtime binaries/plugins for the root package so all ensure entrypoints
	// keep the local runtime in sync with any runtime config changes.
	if pcx.Package.Parent {
		cfg, cfgErr := pcx.Package.GetRuntimeConfig(pcx.Runtime)
		if cfgErr != nil {
			return errors.Wrap(cfgErr, "failed to get runtime config")
		}
		cfg.WorkingDir = pcx.Package.LocalPath
		cfg.Platform = pcx.Platform
		cfg.Format = pcx.Package.Format

		cfg.PluginDeps, err = pcx.GatherPlugins()
		if err != nil {
			return
		}

		pcx.ActualRuntime = cfg

		if err := fs.EnsurePackageLayout(cfg.WorkingDir, cfg.IsOpenMP()); err != nil {
			return errors.Wrap(err, "failed to ensure package layout")
		}

		if err := runtime.EnsureBinaries(pcx.CacheDir, cfg); err != nil {
			return errors.Wrap(err, "failed to ensure runtime binaries")
		}

		if err := runtime.EnsurePlugins(ctx, pcx.GitHub, &pcx.ActualRuntime, pcx.CacheDir, false); err != nil {
			return errors.Wrap(err, "failed to ensure runtime plugins")
		}

		pcx.recordRuntimeToLockfile()
	}

	return err
}

// GatherPlugins iterates the AllPlugins list and appends them to the runtime dependencies list
func (pcx *PackageContext) GatherPlugins() (pluginDeps []versioning.DependencyMeta, err error) {
	print.Verb(pcx.Package, "gathering", len(pcx.AllPlugins), "plugins from package context")
	for _, pluginMeta := range pcx.AllPlugins {
		print.Verb("read plugin from dependency:", pluginMeta)
		pluginDeps = append(pluginDeps, pluginMeta)
	}
	print.Verb(pcx.Package, "gathered plugins:", pluginDeps)
	return
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version.
// When lockfile support is enabled, it uses locked versions for reproducibility.
func (pcx *PackageContext) EnsurePackage(meta versioning.DependencyMeta, forceUpdate bool) error {
	// Handle URL-like schemes differently
	if meta.IsURLScheme() {
		return pcx.ensureURLSchemeDependency(meta)
	}

	// Apply locked version if lockfile is enabled and not forcing update
	effectiveMeta := meta
	if pcx.LockfileResolver != nil && !forceUpdate {
		effectiveMeta = pcx.LockfileResolver.GetLockedVersion(meta)
	}

	dependencyPath := filepath.Join(pcx.Package.Vendor, effectiveMeta.Repo)

	if fs.Exists(dependencyPath) {
		valid, validationErr := ValidateRepository(dependencyPath)
		if validationErr != nil || !valid {
			print.Verb(effectiveMeta, "existing repository is invalid or corrupted")
			if validationErr != nil {
				print.Verb(effectiveMeta, "validation error:", validationErr)
			}
			print.Verb(effectiveMeta, "removing invalid repository for fresh clone")
			err := os.RemoveAll(dependencyPath)
			if err != nil {
				return errors.Wrap(err, "failed to remove invalid dependency repo")
			}
		}
	}

	repo, err := pcx.ensureDependencyRepository(effectiveMeta, dependencyPath, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependency repository")
	}

	err = pcx.updateRepoStateWithRecovery(repo, effectiveMeta, dependencyPath, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to update repository state")
	}

	// Record the resolution to lockfile
	if pcx.LockfileResolver != nil {
		if recordErr := pcx.LockfileResolver.RecordResolution(meta, repo, false, ""); recordErr != nil {
			print.Warn("failed to record dependency resolution to lockfile:", recordErr)
		}
	}

	err = pcx.installPackageResources(effectiveMeta)
	if err != nil {
		return errors.Wrap(err, "failed to install package resources")
	}

	return nil
}

// EnsurePackageWithParent ensures a package and records it as a transitive dependency
func (pcx *PackageContext) EnsurePackageWithParent(meta versioning.DependencyMeta, forceUpdate bool, parentRepo string) error {
	// Handle URL-like schemes differently
	if meta.IsURLScheme() {
		return pcx.ensureURLSchemeDependency(meta)
	}

	// Apply locked version if lockfile is enabled and not forcing update
	effectiveMeta := meta
	if pcx.LockfileResolver != nil && !forceUpdate {
		effectiveMeta = pcx.LockfileResolver.GetLockedVersion(meta)
	}

	dependencyPath := filepath.Join(pcx.Package.Vendor, effectiveMeta.Repo)

	if util.Exists(dependencyPath) {
		valid, validationErr := ValidateRepository(dependencyPath)
		if validationErr != nil || !valid {
			print.Verb(effectiveMeta, "existing repository is invalid or corrupted")
			err := os.RemoveAll(dependencyPath)
			if err != nil {
				return errors.Wrap(err, "failed to remove invalid dependency repo")
			}
		}
	}

	repo, err := pcx.ensureDependencyRepository(effectiveMeta, dependencyPath, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependency repository")
	}

	err = pcx.updateRepoStateWithRecovery(repo, effectiveMeta, dependencyPath, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to update repository state")
	}

	// Record the resolution to lockfile as transitive dependency
	if pcx.LockfileResolver != nil {
		isTransitive := parentRepo != "" && parentRepo != pcx.Package.Repo
		if recordErr := pcx.LockfileResolver.RecordResolution(meta, repo, isTransitive, parentRepo); recordErr != nil {
			print.Warn("failed to record dependency resolution to lockfile:", recordErr)
		}
	}

	err = pcx.installPackageResources(effectiveMeta)
	if err != nil {
		return errors.Wrap(err, "failed to install package resources")
	}

	return nil
}

// GetResolvedCommit returns the resolved commit SHA for a dependency path
func (pcx *PackageContext) GetResolvedCommit(dependencyPath string) (string, error) {
	repo, err := git.PlainOpen(dependencyPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to open repository")
	}

	head, err := repo.Head()
	if err != nil {
		return "", errors.Wrap(err, "failed to get HEAD")
	}

	return head.Hash().String(), nil
}

// installPackageResources handles resource installation from cached package
func (pcx *PackageContext) installPackageResources(meta versioning.DependencyMeta) error {
	// NOTE: Resource installation needs a package definition (`pawn.json`/`pawn.yaml`).
	// We prefer the cached copy because it is typically the newest, but some repos may not have a
	// definition on their default branch (e.g. definition only exists on another branch), or a user may
	// have an older cached clone on a branch that used to exist.
	// To avoid regressions where resource include paths silently disappear, fall back to the checked-out
	// dependency copy and finally the remote package definition.
	pkg, err := pawnpackage.GetCachedPackage(meta, pcx.CacheDir)
	if err != nil {
		print.Verb(meta, "failed to read cached package definition:", err)
	}
	if err != nil || pkg.Format == "" {
		depDir := filepath.Join(pcx.Package.Vendor, meta.Repo)
		pkgLocal, errLocal := pawnpackage.PackageFromDir(depDir)
		if errLocal == nil && pkgLocal.Format != "" {
			pkg = pkgLocal
			err = nil
			print.Verb(meta, "using local dependency package definition for resources")
		} else if pcx.GitHub != nil {
			pkgRemote, errRemote := pawnpackage.GetRemotePackage(context.Background(), pcx.GitHub, meta)
			if errRemote == nil {
				pkg = pkgRemote
				err = nil
				print.Verb(meta, "using remote package definition for resources")
			}
		}
	}

	// But the cached copy will have the latest tag assigned to it, so before ensuring it, apply the
	// tag of the actual package we installed.
	pkg.Tag = meta.Tag

	var includePath string
	for _, resource := range pkg.Resources {
		if resource.Platform != pcx.Platform {
			continue
		}

		if len(resource.Includes) > 0 {
			includePath, err = pcx.extractResourceDependencies(context.Background(), pkg, resource)
			if err != nil {
				return err
			}
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includePath)
		}
	}

	return err
}

// ensureURLSchemeDependency handles dependencies with URL-like schemes (plugin://, includes://, filterscript://)
func (pcx *PackageContext) ensureURLSchemeDependency(meta versioning.DependencyMeta) error {
	return ensureURLSchemeWithHandler(pcx, meta)
}

// ensurePluginDependency handles plugin:// scheme dependencies
func (pcx *PackageContext) ensurePluginDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local plugin: plugin://local/path
		pluginPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !fs.Exists(pluginPath) {
			return errors.Errorf("local plugin path does not exist: %s", pluginPath)
		}

		pluginMeta := versioning.DependencyMeta{
			Scheme: "plugin",
			Local:  meta.Local,
			User:   "local",
			Repo:   filepath.Base(meta.Local),
		}

		pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
		print.Verb(meta, "added local plugin dependency:", pluginPath)
		return nil
	}

	// Remote plugin: plugin://user/repo or plugin://user/repo:tag
	// Treat as a regular dependency but mark it as a plugin
	ensureMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}
	if ensureMeta.Site == "" {
		ensureMeta.Site = "github.com"
	}

	err := pcx.EnsurePackage(ensureMeta, false)
	if err != nil {
		return err
	}

	remoteMeta := ensureMeta
	remoteMeta.Scheme = "plugin"

	pcx.AllPlugins = append(pcx.AllPlugins, remoteMeta)
	print.Verb(meta, "added remote plugin dependency:", remoteMeta)
	return nil
}

// ensureComponentDependency handles component:// scheme dependencies
// Components are installed like plugins but into the ./components directory.
func (pcx *PackageContext) ensureComponentDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local component: component://local/path
		componentPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !fs.Exists(componentPath) {
			return errors.Errorf("local component path does not exist: %s", componentPath)
		}

		componentMeta := versioning.DependencyMeta{
			Scheme: "component",
			Local:  meta.Local,
			User:   "local",
			Repo:   filepath.Base(meta.Local),
		}

		pcx.AllPlugins = append(pcx.AllPlugins, componentMeta)
		print.Verb(meta, "added local component dependency:", componentPath)
		return nil
	}

	// Remote component: component://user/repo or component://user/repo:tag
	ensureMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}
	if ensureMeta.Site == "" {
		ensureMeta.Site = "github.com"
	}

	err := pcx.EnsurePackage(ensureMeta, false)
	if err != nil {
		return err
	}

	remoteMeta := ensureMeta
	remoteMeta.Scheme = "component"

	pcx.AllPlugins = append(pcx.AllPlugins, remoteMeta)
	print.Verb(meta, "added remote component dependency:", remoteMeta)
	return nil
}

// ensureIncludesDependency handles includes:// scheme dependencies
func (pcx *PackageContext) ensureIncludesDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local includes: includes://local/path
		includesPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !fs.Exists(includesPath) {
			return errors.Errorf("local includes path does not exist: %s", includesPath)
		}

		pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
		print.Verb(meta, "added local includes path:", includesPath)
		return nil
	}

	// Remote includes: includes://user/repo or includes://user/repo:tag
	// Ensure the dependency and add its path to includes
	remoteMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}

	err := pcx.EnsurePackage(remoteMeta, false)
	if err != nil {
		return err
	}

	includesPath := filepath.Join(pcx.Package.Vendor, remoteMeta.Repo)
	if remoteMeta.Path != "" {
		includesPath = filepath.Join(includesPath, remoteMeta.Path)
	}
	pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
	print.Verb(meta, "added remote includes path:", includesPath)
	return nil
}

// ensureFilterscriptDependency handles filterscript:// scheme dependencies
func (pcx *PackageContext) ensureFilterscriptDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local filterscript: filterscript://local/path
		filterscriptPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !fs.Exists(filterscriptPath) {
			return errors.Errorf("local filterscript path does not exist: %s", filterscriptPath)
		}

		print.Verb(meta, "added local filterscript dependency:", filterscriptPath)
		return nil
	}

	// Remote filterscript: filterscript://user/repo or filterscript://user/repo:tag
	// Treat as a regular dependency
	remoteMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}

	err := pcx.EnsurePackage(remoteMeta, false)
	if err != nil {
		return err
	}

	print.Verb(meta, "added filterscript dependency:", remoteMeta)
	return nil
}

func (pcx PackageContext) extractResourceDependencies(
	ctx context.Context,
	pkg pawnpackage.Package,
	res resource.Resource,
) (dir string, err error) {
	dir = filepath.Join(pcx.Package.Vendor, res.Path(pkg.Repo))
	print.Verb(pkg, "installing resource-based dependency", res.Name, "to", dir)

	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		err = errors.Wrap(err, "failed to create target directory")
		return
	}

	_, err = runtime.EnsureVersionedPlugin(
		ctx,
		pcx.GitHub,
		pkg.DependencyMeta,
		dir,
		pcx.Platform,
		res.Version,
		pcx.CacheDir,
		"",
		false,
		true,
		false,
		pcx.Package.ExtractIgnorePatterns,
	)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure asset")
		return
	}

	return dir, nil
}

func (pcx *PackageContext) recordRuntimeToLockfile() {
	if pcx.LockfileResolver == nil {
		return
	}

	manifestInfo, err := runtime.GetRuntimeManifestInfo(pcx.Package.LocalPath)
	if err != nil {
		print.Warn("failed to get runtime manifest info:", err)
		return
	}
	if manifestInfo == nil {
		return
	}

	files := make([]lockfile.LockedFileInfo, len(manifestInfo.Files))
	for i, f := range manifestInfo.Files {
		files[i] = lockfile.LockedFileInfo{
			Path: f.Path,
			Size: f.Size,
			Hash: f.Hash,
			Mode: f.Mode,
		}
	}

	pcx.LockfileResolver.RecordRuntime(
		manifestInfo.Version,
		manifestInfo.Platform,
		manifestInfo.RuntimeType,
		files,
	)
}

func (pcx *PackageContext) RecordBuildToLockfile(compilerVersion, compilerPreset, entry, output string) {
	if pcx.LockfileResolver == nil {
		return
	}

	outputHash := ""
	if output != "" && util.Exists(output) {
		hash, err := hashOutputFile(output)
		if err != nil {
			print.Warn("failed to hash output file:", err)
		} else {
			outputHash = hash
		}
	}

	pcx.LockfileResolver.RecordBuild(compilerVersion, compilerPreset, entry, output, outputHash)
}

func hashOutputFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
