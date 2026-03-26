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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		dep := dependency
		r := retrier.New(retrier.ConstantBackoff(1, 100*time.Millisecond), nil)
		err := r.Run(func() error {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}

			print.Verb("attempting to ensure dependency", dep)
			errInner := pcx.ensurePackage(ctx, dep, forceUpdate)
			if errInner != nil {
				print.Warn(errors.Wrapf(errInner, "failed to ensure package %s", dep))
				return errInner
			}
			print.Info(pcx.Package, "successfully ensured dependency files for", dep)
			return nil
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				if ctxErr != nil {
					return ctxErr
				}
				return err
			}

			print.Warn("failed to ensure package", dep, "after 2 attempts, skipping")
			continue
		}
	}

	// Ensure runtime binaries/plugins for the root package so all ensure entrypoints
	// keep the local runtime in sync with any runtime config changes.
	if pcx.Package.Parent {
		if err := pcx.ensureParentRuntime(ctx); err != nil {
			return err
		}
	}

	return err
}

// EnsureProject applies the full project ensure flow used by user-facing commands.
// It pins tagless dependencies where possible, ensures dependency/runtime files,
// and persists the lockfile when lockfile support is enabled.
func (pcx *PackageContext) EnsureProject(ctx context.Context, forceUpdate bool) (bool, error) {
	updated, err := pcx.TagTaglessDependencies(ctx, forceUpdate)
	if err != nil {
		return false, err
	}

	if err := pcx.EnsureDependencies(ctx, forceUpdate); err != nil {
		return updated, err
	}

	deps, err := pcx.currentLockfileDependencies()
	if err != nil {
		return updated, err
	}
	pcx.recordRootLocalDependencies()
	pcx.pruneLockfileDependencies(lockfileDependencyMetas(deps))

	if err := pcx.PackageLockfileState.SaveLockfile(); err != nil {
		return updated, err
	}

	return updated, nil
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

// ensureURLSchemeDependency handles dependencies with URL-like schemes (plugin://, includes://, filterscript://)
func (pcx *PackageContext) ensureURLSchemeDependency(ctx context.Context, meta versioning.DependencyMeta) error {
	return ensureURLSchemeWithHandler(ctx, pcx, meta)
}

// ensurePluginDependency handles plugin:// scheme dependencies
func (pcx *PackageContext) ensurePluginDependency(ctx context.Context, meta versioning.DependencyMeta) error {
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

	err := pcx.ensurePackage(ctx, ensureMeta, false)
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
func (pcx *PackageContext) ensureComponentDependency(ctx context.Context, meta versioning.DependencyMeta) error {
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

	err := pcx.ensurePackage(ctx, ensureMeta, false)
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
func (pcx *PackageContext) ensureIncludesDependency(ctx context.Context, meta versioning.DependencyMeta) error {
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

	err := pcx.ensurePackage(ctx, remoteMeta, false)
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
func (pcx *PackageContext) ensureFilterscriptDependency(ctx context.Context, meta versioning.DependencyMeta) error {
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

	err := pcx.ensurePackage(ctx, remoteMeta, false)
	if err != nil {
		return err
	}

	print.Verb(meta, "added filterscript dependency:", remoteMeta)
	return nil
}

func hashOutputFile(path string) (hash string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
