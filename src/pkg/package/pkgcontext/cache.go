package pkgcontext

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type repoEnsureRequest struct {
	Context     context.Context
	From        string
	To          string
	Branch      string
	SSH         bool
	FullClone   bool
	ForceUpdate bool
}

type repoEnsureWithMetaRequest struct {
	Meta versioning.DependencyMeta
	repoEnsureRequest
}

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func (pcx *PackageContext) EnsureDependenciesCached() error {
	return pcx.EnsureDependenciesCachedContext(context.Background())
}

// EnsureDependenciesCachedContext visits dependencies in the shared cache and respects cancellation.
func (pcx *PackageContext) EnsureDependenciesCachedContext(ctx context.Context) error {
	return pcx.refreshDependencyGraphContext(ctx, DependencyUpdateRequest{})
}

func (pcx *PackageContext) refreshDependencyGraph(request DependencyUpdateRequest) error {
	return pcx.refreshDependencyGraphContext(context.Background(), request)
}

func (pcx *PackageContext) refreshDependencyGraphContext(ctx context.Context, request DependencyUpdateRequest) error {
	return pcx.refreshDependencyGraphWithResolution(ctx, request, true)
}

// Reference-only updates can name tags that are not cached yet.
func (pcx *PackageContext) refreshDependencyReferenceGraphContext(ctx context.Context, request DependencyUpdateRequest) error {
	return pcx.refreshDependencyGraphWithResolution(ctx, request, false)
}

func (pcx *PackageContext) refreshDependencyGraphWithResolution(ctx context.Context, request DependencyUpdateRequest, resolveLatest bool) error {
	if !pcx.Package.Parent {
		return errors.New("package is not a parent package")
	}
	if pcx.Package.LocalPath == "" {
		return errors.New("package has no known local path")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	pcx.AllDependencies = nil
	pcx.AllPlugins = nil
	pcx.AllIncludePaths = nil
	visited := map[versioning.DependencyMeta]bool{managedDependencyIdentity(pcx.Package.Dependency()): true}
	directDependencies := make(map[versioning.DependencyMeta]bool)
	directConstraints := pcx.directDependencyConstraints()
	for _, declaration := range pcx.Package.GetAllDependencies() {
		meta, err := declaration.Explode()
		if err == nil {
			directDependencies[managedDependencyIdentity(meta)] = true
		}
	}
	destinations := make(map[string]versioning.DependencyMeta)
	var recurse func(versioning.DependencyMeta, bool, int) error
	recurse = func(meta versioning.DependencyMeta, direct bool, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		prefix := strings.Repeat("|-", depth+1)
		currentPackage := pcx.Package
		if depth != 0 {
			forceUpdate := request.ShouldForceDependency(meta, direct || directDependencies[managedDependencyIdentity(meta)])
			effectiveMeta := pcx.LockedVersion(meta, forceUpdate)
			if resolveLatest {
				resolvedMeta, err := pcx.resolveDynamicDependencyReference(ctx, effectiveMeta, meta, forceUpdate)
				if err != nil {
					return &DependencyError{Dependency: meta.String(), Operation: "resolve", Err: err}
				}
				effectiveMeta = resolvedMeta
			}
			refreshCache := forceUpdate
			if refreshCache {
				noOrigin, err := pcx.cachedRepoHasNoOriginContext(ctx, effectiveMeta)
				if err != nil {
					return &DependencyError{Dependency: meta.String(), Operation: "cache", Err: err}
				}
				refreshCache = !noOrigin
			}
			if _, err := pcx.EnsureDependencyCachedContext(ctx, effectiveMeta, refreshCache); err != nil {
				return &DependencyError{Dependency: meta.String(), Operation: "cache", Err: err}
			}
			var err error
			currentPackage, err = pcx.packageFromCachedRevisionContext(ctx, effectiveMeta, effectiveMeta.CachePath(pcx.CacheDir))
			if err != nil {
				return &DependencyError{Dependency: meta.String(), Operation: "read package metadata", Err: err}
			}
			pcx.AllDependencies = append(pcx.AllDependencies, meta)
			pcx.recordCachedPackageResources(currentPackage, meta, prefix)
		}

		dependencies := currentPackage.Dependencies
		if currentPackage.Parent {
			dependencies = currentPackage.GetAllDependencies()
		}
		var failures []error
		for _, dependency := range dependencies {
			if err := ctx.Err(); err != nil {
				return stderrors.Join(append(failures, err)...)
			}
			subMeta, err := dependency.Explode()
			if err != nil {
				failures = append(failures, &DependencyError{Dependency: string(dependency), Operation: "parse", Err: errors.Wrapf(err, "invalid dependency in %s", currentPackage)})
				continue
			}
			subMeta = inheritDirectDependencyConstraint(subMeta, directConstraints)
			if !subMeta.IsLocalScheme() {
				if err := registerDependencyDestination(destinations, subMeta); err != nil {
					failures = append(failures, err)
					continue
				}
			}
			if subMeta.IsURLScheme() {
				err = pcx.handleURLSchemeCaching(subMeta, prefix)
			} else if identity := managedDependencyIdentity(subMeta); !visited[identity] {
				visited[identity] = true
				err = recurse(subMeta, depth == 0, depth+1)
			}
			if err != nil {
				var dependencyErr *DependencyError
				if !stderrors.As(err, &dependencyErr) {
					err = &DependencyError{Dependency: subMeta.String(), Operation: "cache", Err: err}
				}
				failures = append(failures, err)
				if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
					return stderrors.Join(failures...)
				}
			}
		}
		return stderrors.Join(failures...)
	}
	return recurse(pcx.Package.Dependency(), false, 0)
}

func (pcx *PackageContext) recordCachedPackageResources(pkg pawnpackage.Package, meta versioning.DependencyMeta, prefix string) {
	isPlugin := false
	for _, resource := range pkg.Resources {
		if resource.Platform == pcx.Platform && len(resource.Includes) > 0 {
			targetPath := filepath.Join(pcx.Package.Vendor, resource.Path(pkg.Repo))
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, targetPath)
			print.Verb(prefix, "added target path for resource includes:", targetPath)
		}
		if resource.Archive {
			isPlugin = isPlugin || len(resource.Plugins) > 0
		} else {
			isPlugin = isPlugin || strings.Contains(resource.Name, "dll") || strings.Contains(resource.Name, "so")
		}
	}
	if isPlugin {
		pcx.AllPlugins = append(pcx.AllPlugins, meta)
	}
}

func (pcx PackageContext) packageFromCachedRevision(
	meta versioning.DependencyMeta,
	path string,
) (pawnpackage.Package, error) {
	return pcx.packageFromCachedRevisionContext(context.Background(), meta, path)
}

func (pcx PackageContext) readPackageFromCachedRevision(meta versioning.DependencyMeta, path string) (pawnpackage.Package, error) {
	if (meta.Tag == "" || meta.Tag == "latest") && meta.Commit == "" {
		return pawnpackage.PackageFromDir(path)
	}

	repo, err := pcx.PackageServices.repositoryStore().Open(path)
	if err != nil {
		return pawnpackage.Package{}, err
	}

	var ref *plumbing.Reference
	if meta.Tag != "" {
		ref, err = versioning.RefFromTag(repo, meta)
	} else {
		ref, err = versioning.RefFromCommit(repo, meta)
	}
	if err != nil {
		return pawnpackage.Package{}, err
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return pawnpackage.Package{}, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return pawnpackage.Package{}, err
	}

	var (
		pkg      pawnpackage.Package
		found    bool
		lastPath string
	)
	for _, definition := range []struct {
		path   string
		format string
	}{
		{path: "pawn.json", format: "json"},
		{path: "pawn.yaml", format: "yaml"},
	} {
		file, fileErr := tree.File(definition.path)
		if fileErr == object.ErrFileNotFound {
			continue
		}
		if fileErr != nil {
			return pawnpackage.Package{}, fileErr
		}
		if found {
			return pawnpackage.Package{}, errors.Errorf(
				"found both %s and %s; please keep only one package definition file",
				lastPath,
				definition.path,
			)
		}
		contents, contentErr := file.Contents()
		if contentErr != nil {
			return pawnpackage.Package{}, contentErr
		}
		pkg, err = pawnpackage.PackageFromDefinition([]byte(contents), definition.format)
		if err != nil {
			return pawnpackage.Package{}, err
		}
		found = true
		lastPath = definition.path
	}

	return pkg, nil
}

// handleURLSchemeCaching handles URL scheme dependencies during the caching phase
func (pcx *PackageContext) handleURLSchemeCaching(meta versioning.DependencyMeta, prefix string) error {
	switch meta.Scheme {
	case "plugin":
		if meta.IsLocalScheme() {
			pluginMeta := versioning.DependencyMeta{
				Scheme: "plugin",
				Local:  meta.Local,
				User:   "local",
				Repo:   filepath.Base(meta.Local),
			}
			pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
			print.Verb(prefix, "added local plugin:", meta.Local)
		} else {
			ensureMeta := versioning.DependencyMeta{
				Site:   meta.Site,
				User:   meta.User,
				Repo:   meta.Repo,
				Tag:    meta.Tag,
				Branch: meta.Branch,
				Commit: meta.Commit,
				Path:   meta.Path,
			}
			pcx.AllDependencies = append(pcx.AllDependencies, ensureMeta)

			pluginMeta := ensureMeta
			pluginMeta.Scheme = "plugin"
			pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
			print.Verb(prefix, "added remote plugin dependency:", ensureMeta)
		}

	case "component":
		if meta.IsLocalScheme() {
			componentMeta := versioning.DependencyMeta{
				Scheme: "component",
				Local:  meta.Local,
				User:   "local",
				Repo:   filepath.Base(meta.Local),
			}
			pcx.AllPlugins = append(pcx.AllPlugins, componentMeta)
			print.Verb(prefix, "added local component:", meta.Local)
		} else {
			ensureMeta := versioning.DependencyMeta{
				Site:   meta.Site,
				User:   meta.User,
				Repo:   meta.Repo,
				Tag:    meta.Tag,
				Branch: meta.Branch,
				Commit: meta.Commit,
				Path:   meta.Path,
			}
			pcx.AllDependencies = append(pcx.AllDependencies, ensureMeta)

			componentMeta := ensureMeta
			componentMeta.Scheme = "component"
			pcx.AllPlugins = append(pcx.AllPlugins, componentMeta)
			print.Verb(prefix, "added remote component dependency:", ensureMeta)
		}

	case "includes":
		if meta.IsLocalScheme() {
			includesPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
			print.Verb(prefix, "added local includes path:", includesPath)
		} else {
			remoteMeta := versioning.DependencyMeta{
				Site:   meta.Site,
				User:   meta.User,
				Repo:   meta.Repo,
				Tag:    meta.Tag,
				Branch: meta.Branch,
				Commit: meta.Commit,
				Path:   meta.Path,
			}
			pcx.AllDependencies = append(pcx.AllDependencies, remoteMeta)

			includesPath := filepath.Join(pcx.Package.Vendor, remoteMeta.Repo)
			if remoteMeta.Path != "" {
				includesPath = filepath.Join(includesPath, remoteMeta.Path)
			}
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
			print.Verb(prefix, "added remote includes dependency:", remoteMeta)
		}

	case "filterscript":
		if meta.IsLocalScheme() {
			print.Verb(prefix, "added local filterscript:", meta.Local)
		} else {
			remoteMeta := versioning.DependencyMeta{
				Site:   meta.Site,
				User:   meta.User,
				Repo:   meta.Repo,
				Tag:    meta.Tag,
				Branch: meta.Branch,
				Commit: meta.Commit,
				Path:   meta.Path,
			}
			pcx.AllDependencies = append(pcx.AllDependencies, remoteMeta)
			print.Verb(prefix, "added remote filterscript dependency:", remoteMeta)
		}

	default:
		return errors.Errorf("unsupported URL scheme: %s", meta.Scheme)
	}

	return nil
}

// EnsureDependencyFromCache ensures the repository at `path` is up to date
func (pcx PackageContext) EnsureDependencyFromCache(
	meta versioning.DependencyMeta,
	path string,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	return pcx.EnsureDependencyFromCacheContext(context.Background(), meta, path, forceUpdate)
}

// EnsureDependencyFromCacheContext installs a cached repository while respecting cancellation.
func (pcx PackageContext) EnsureDependencyFromCacheContext(ctx context.Context, meta versioning.DependencyMeta, path string, forceUpdate bool) (repo *git.Repository, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	print.Verb(meta, "ensuring dependency package from cache to", path, "force update:", forceUpdate)

	from, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		err = errors.Wrap(err, "failed to make canonical path to cached copy")
		return
	}
	cacheRepoExists := fs.Exists(filepath.Join(from, ".git"))
	if !cacheRepoExists || forceUpdate {
		if forceUpdate && cacheRepoExists {
			noOrigin, originErr := pcx.cachedRepoHasNoOriginContext(ctx, meta)
			if originErr != nil {
				return nil, originErr
			}
			if noOrigin {
				print.Verb(meta, "cached repository has no origin remote, skipping cache refresh")
			} else {
				_, err = pcx.EnsureDependencyCachedContext(ctx, meta, forceUpdate)
				if err != nil {
					return
				}
			}
		} else {
			_, err = pcx.EnsureDependencyCachedContext(ctx, meta, forceUpdate)
			if err != nil {
				return
			}
		}
	}
	unlock, err := lockRepositoryPath(ctx, from)
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()
	if err := validateRepositoryDestination(from); err != nil {
		return nil, err
	}
	if destination, absErr := filepath.Abs(path); absErr != nil {
		return nil, absErr
	} else if destination == from {
		return pcx.PackageServices.repositoryStore().Open(from)
	}

	repo, err = pcx.ensureRepoExistsWithMeta(repoEnsureWithMetaRequest{
		Meta: meta,
		repoEnsureRequest: repoEnsureRequest{
			Context:     ctx,
			From:        from,
			To:          path,
			Branch:      meta.Branch,
			SSH:         meta.SSH != "",
			FullClone:   meta.Tag != "" || meta.Commit != "",
			ForceUpdate: forceUpdate,
		},
	})
	return
}

// EnsureDependencyCached clones a package to path using the default branch
func (pcx PackageContext) EnsureDependencyCached(
	meta versioning.DependencyMeta,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	return pcx.EnsureDependencyCachedContext(context.Background(), meta, forceUpdate)
}

// EnsureDependencyCachedContext retrieves a cached repository while respecting cancellation.
func (pcx PackageContext) EnsureDependencyCachedContext(ctx context.Context, meta versioning.DependencyMeta, forceUpdate bool) (*git.Repository, error) {
	to, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		return nil, err
	}
	return pcx.ensureRepoExistsWithMeta(repoEnsureWithMetaRequest{
		Meta: meta,
		repoEnsureRequest: repoEnsureRequest{
			Context:     ctx,
			From:        meta.URL(),
			To:          to,
			Branch:      meta.Branch,
			SSH:         meta.SSH != "",
			FullClone:   meta.Tag != "" || meta.Commit != "",
			ForceUpdate: forceUpdate,
		},
	})
}

func (pcx PackageContext) ensureRepoExists(request repoEnsureRequest) (repo *git.Repository, err error) {
	if request.Context == nil {
		request.Context = context.Background()
	}
	if err := request.Context.Err(); err != nil {
		return nil, err
	}
	if err := validateRepositoryDestination(request.To); err != nil {
		return nil, err
	}
	unlock, err := lockRepositoryPath(request.Context, request.To)
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()
	if err := validateRepositoryDestination(request.To); err != nil {
		return nil, err
	}

	if fs.Exists(request.To) {
		valid, validationErr := pcx.PackageServices.repositoryHealth().Validate(request.To)
		if validationErr != nil {
			return nil, errors.Wrap(validationErr, "failed to validate repository")
		}
		if !valid {
			return pcx.cloneRepositoryContext(request)
		}
	}

	repo, err = pcx.PackageServices.repositoryStore().Open(request.To)
	if err != nil {
		if !stderrors.Is(err, git.ErrRepositoryNotExists) && !stderrors.Is(err, os.ErrNotExist) {
			return nil, errors.Wrap(err, "failed to open repository")
		}
		return pcx.cloneRepositoryContext(request)
	}
	if request.ForceUpdate {
		return pcx.updateRepositoryStaged(request)
	}
	return repo, nil
}

func (pcx PackageContext) cloneRepository(from, to, branch string, ssh bool, fullClone bool) (*git.Repository, error) {
	return pcx.cloneRepositoryContext(repoEnsureRequest{Context: context.Background(), From: from, To: to, Branch: branch, SSH: ssh, FullClone: fullClone})
}

func (pcx PackageContext) updateRepository(repo *git.Repository, request repoEnsureRequest) (*git.Repository, error) {
	if request.Context == nil {
		request.Context = context.Background()
	}
	wt, err := repo.Worktree()
	if err != nil {
		return pcx.cloneRepositoryContext(request)
	}
	pullOpts := &git.PullOptions{Auth: pcx.authForRemote(request.From, request.SSH)}
	if request.Branch != "" {
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + request.Branch)
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = wt.PullContext(request.Context, pullOpts)
		if err == nil || stderrors.Is(err, git.NoErrAlreadyUpToDate) {
			return repo, nil
		}
		if !isTransientGitError(err) || attempt == 2 {
			break
		}
		if err := waitGitRetry(request.Context, attempt); err != nil {
			return nil, err
		}
	}
	if ctxErr := request.Context.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) || !isRepositoryCorrupt(request.To) {
		return nil, errors.Wrap(err, "failed to update repository")
	}
	return pcx.cloneRepositoryContext(request)
}

func (pcx PackageContext) recoverByReclone(from, to, branch string, ssh bool, fullClone bool) (*git.Repository, error) {
	return pcx.cloneRepository(from, to, branch, ssh, fullClone)
}

// ensureRepoExistsWithMeta wraps ensureRepoExists and provides improved error handling
func (pcx PackageContext) ensureRepoExistsWithMeta(request repoEnsureWithMetaRequest) (repo *git.Repository, err error) {
	repo, err = pcx.ensureRepoExists(request.repoEnsureRequest)
	if err != nil && pcx.shouldRetryWithSSH(request.Meta, request.From, request.SSH, err) {
		fallbackFrom := toGitSSHURL(request.Meta)
		print.Verb(request.Meta, "HTTPS git operation failed, retrying with SSH:", fallbackFrom)
		repo, err = pcx.ensureRepoExists(repoEnsureRequest{
			Context:     request.Context,
			From:        fallbackFrom,
			To:          request.To,
			Branch:      request.Branch,
			SSH:         true,
			FullClone:   request.FullClone,
			ForceUpdate: request.ForceUpdate,
		})
	}
	if err != nil {
		err = WrapGitError(err, request.Meta)
	}
	return repo, err
}

func (pcx PackageContext) shouldRetryWithSSH(meta versioning.DependencyMeta, from string, ssh bool, err error) bool {
	if err == nil || ssh || pcx.GitAuth == nil || stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if !strings.HasPrefix(from, "https://") && !strings.HasPrefix(from, "http://") {
		return false
	}

	site := meta.Site
	if site == "" {
		site = "github.com"
	}
	if site != "github.com" {
		return false
	}

	if meta.User == "" || meta.Repo == "" {
		return false
	}

	if pcx.authForRemote(toGitSSHURL(meta), true) == nil {
		return false
	}

	return isGitAuthenticationError(err)
}

func toGitSSHURL(meta versioning.DependencyMeta) string {
	site := meta.Site
	if site == "" {
		site = "github.com"
	}
	user := meta.SSH
	if user == "" {
		user = "git"
	}
	return fmt.Sprintf("%s@%s:%s/%s", user, site, meta.User, meta.Repo)
}

func isGitAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "authentication required") ||
		strings.Contains(errStr, "repository not found") ||
		strings.Contains(errStr, "could not read from remote repository") ||
		strings.Contains(errStr, "permission denied")
}

func (pcx PackageContext) authForRemote(from string, ssh bool) transport.AuthMethod {
	if pcx.GitAuth == nil {
		return nil
	}

	if authBundle, ok := pcx.GitAuth.(*GitMultiAuth); ok {
		if ssh || isSSHRemote(from) {
			if authBundle.SSH != nil {
				return authBundle.SSH
			}
			return nil
		}

		if strings.HasPrefix(from, "https://") || strings.HasPrefix(from, "http://") {
			if authBundle.HTTP != nil {
				return authBundle.HTTP
			}
			return nil
		}

		return nil
	}

	authName := pcx.GitAuth.Name()

	if ssh || isSSHRemote(from) {
		if strings.HasPrefix(authName, "ssh-") {
			return pcx.GitAuth
		}
		return nil
	}

	if strings.HasPrefix(from, "https://") || strings.HasPrefix(from, "http://") {
		if strings.HasPrefix(authName, "http-") {
			return pcx.GitAuth
		}
		return nil
	}

	return nil
}

func isSSHRemote(from string) bool {
	if strings.HasPrefix(from, "ssh://") || strings.HasPrefix(from, "git@") {
		return true
	}
	return strings.Contains(from, "@") && strings.Contains(from, ":")
}
