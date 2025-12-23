package pkgcontext

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func (pcx *PackageContext) EnsureDependenciesCached() (errOuter error) {
	if !pcx.Package.Parent {
		errOuter = errors.New("package is not a parent package")
		return
	}
	if pcx.Package.LocalPath == "" {
		errOuter = errors.New("package has no known local path")
		return
	}

	// This recursive operation requires quite a lot of state! There is probably
	// a better method to break this up but so far, this has worked fine.
	var (
		recurse        func(meta versioning.DependencyMeta)
		visited        = make(map[string]bool)
		dependencyPath = pcx.Package.LocalPath
		firstIter      = true
		currentPackage pawnpackage.Package
		errInner       error
	)

	// clear the dependencies list in case this function is being called on an
	// already initialised context that already has some dependencies listed.
	pcx.AllDependencies = nil

	// set the parent package visited state to true, just in case it depends on
	// itself or a dependency depends on it. This should never happen but if it
	// does, this prevents an infinite recursion.
	visited[pcx.Package.DependencyMeta.Repo] = true

	// keep track of recursion depth
	verboseDepth := 0

	recurse = func(currentMeta versioning.DependencyMeta) {
		// this makes visualising the dependency tree easier with --verbose
		verboseDepth++
		prefix := strings.Repeat("|-", verboseDepth)

		// the first iteration of this recursive function is called on the
		// parent package. This means it does not need to be cloned to the cache
		// and the path will be it's true, user-defined location.
		if firstIter {
			currentPackage = pcx.Package // set the current package to the parent
			print.Verb(prefix, currentPackage, "is parent")
		} else {
			dependencyPath = currentMeta.CachePath(pcx.CacheDir)

			_, errInner = pcx.EnsureDependencyCached(currentMeta, false)
			if errInner != nil {
				print.Erro(errInner)
				return
			}
			pcx.AllDependencies = append(pcx.AllDependencies, currentMeta)
			print.Verb(prefix, currentMeta, "ensured")

			currentPackage, errInner = pawnpackage.PackageFromDir(dependencyPath)
			if errInner != nil {
				print.Verb(prefix, currentMeta, "is not a package:", errInner)
				return
			}

			// Run through resources for the target platform and grab all the
			// include paths that will be used for includes from resource archives.
			for _, res := range currentPackage.Resources {
				if res.Platform != pcx.Platform {
					print.Verb(prefix, "ignoring platform mismatch", res.Platform)
					continue
				}

				if len(res.Includes) > 0 {
					targetPath := filepath.Join(pcx.Package.Vendor, res.Path(currentPackage.Repo))
					pcx.AllIncludePaths = append(pcx.AllIncludePaths, targetPath)
					print.Verb(prefix, "added target path for resource includes:", targetPath)
				}
			}

			// some resources may not be plugins
			isPlugin := false

			for _, resource := range currentPackage.Resources {
				if resource.Archive {
					if len(resource.Plugins) > 0 {
						isPlugin = true
					}
				} else {
					if strings.Contains(resource.Name, "dll") || strings.Contains(resource.Name, "so") {
						isPlugin = true
					}
				}
			}

			if isPlugin {
				pcx.AllPlugins = append(pcx.AllPlugins, currentMeta)
				print.Verb(prefix, currentMeta, "is a plugin")
			}
		}

		// mark the repo as visited so we don't hit it again in case it appears
		// multiple times within the dependency tree.
		visited[currentMeta.Repo] = true

		// first iteration has finished, mark it false and next iterations will
		// operate on dependencies
		firstIter = false

		var subPackageDepStrings []versioning.DependencyString
		if currentPackage.Parent {
			subPackageDepStrings = currentPackage.GetAllDependencies()
		} else {
			subPackageDepStrings = currentPackage.Dependencies
		}

		print.Verb(prefix, "iterating", len(subPackageDepStrings), "dependencies of", currentPackage)
		var subPackageDepMeta versioning.DependencyMeta
		for _, subPackageDepString := range subPackageDepStrings {
			subPackageDepMeta, errInner = subPackageDepString.Explode()
			if errInner != nil {
				print.Verb(prefix, "invalid dependency string:", subPackageDepMeta, "in", currentPackage, errInner)
				continue
			}

			// Handle URL-like schemes during caching phase
			if subPackageDepMeta.IsURLScheme() {
				errInner = pcx.handleURLSchemeCaching(subPackageDepMeta, prefix)
				if errInner != nil {
					print.Verb(prefix, "failed to handle URL scheme dependency:", subPackageDepMeta, errInner)
					continue
				}
			} else {
				// Regular dependency handling
				if _, ok := visited[subPackageDepMeta.Repo]; !ok {
					recurse(subPackageDepMeta)
				} else {
					print.Verb(prefix, "already visited", subPackageDepMeta)
				}
			}
		}
		verboseDepth--
	}
	recurse(pcx.Package.DependencyMeta)

	if errInner != nil {
		return errors.New("Failed to clone the repo")
	}

	return nil
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
	print.Verb(meta, "ensuring dependency package from cache to", path, "force update:", forceUpdate)

	from, err := filepath.Abs(meta.CachePath(pcx.CacheDir))
	if err != nil {
		err = errors.Wrap(err, "failed to make canonical path to cached copy")
		return
	}
	if !fs.Exists(filepath.Join(from, ".git")) || forceUpdate {
		_, err = pcx.EnsureDependencyCached(meta, forceUpdate)
		if err != nil {
			return
		}
	}

	repo, err = pcx.ensureRepoExistsWithMeta(meta, from, path, meta.Branch, meta.SSH != "", forceUpdate)
	return
}

// EnsureDependencyCached clones a package to path using the default branch
func (pcx PackageContext) EnsureDependencyCached(
	meta versioning.DependencyMeta,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	return pcx.ensureRepoExistsWithMeta(meta, meta.URL(), meta.CachePath(pcx.CacheDir), meta.Branch, meta.SSH != "", forceUpdate)
}

func (pcx PackageContext) ensureRepoExists(
	from,
	to,
	branch string,
	ssh,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	if fs.Exists(to) {
		valid, validationErr := ValidateRepository(to)
		if validationErr != nil || !valid {
			print.Verb("repository at", to, "is invalid or corrupted")
			if validationErr != nil {
				print.Verb("validation error:", validationErr)
			}
			print.Verb("removing invalid repository for fresh clone")
			errRemove := os.RemoveAll(to)
			if errRemove != nil {
				return nil, errors.Wrap(errRemove, "failed to remove invalid repository")
			}
		}
	}

	repo, err = git.PlainOpen(to)
	if err != nil {
		// Repository doesn't exist or can't be opened - clone it
		return pcx.cloneRepository(from, to, branch, ssh)
	}

	if forceUpdate {
		return pcx.updateRepository(repo, to, branch, ssh, from)
	}

	return repo, nil
}

// cloneRepository performs a fresh clone with validation
func (pcx PackageContext) cloneRepository(from, to, branch string, ssh bool) (*git.Repository, error) {
	print.Verb("cloning repository from", from, "to", to)

	if fs.Exists(to) {
		print.Verb("removing existing directory before clone")
		err := os.RemoveAll(to)
		if err != nil {
			return nil, errors.Wrap(err, "failed to remove existing directory")
		}
	}

	err := os.MkdirAll(to, 0o700)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create directory for clone")
	}

	// Configure clone options
	cloneOpts := &git.CloneOptions{
		URL:   from,
		Depth: 1000, // TODO: We might want to consider removing depth for better reliability, or add a configurable option
	}
	if branch != "" {
		cloneOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + branch)
	}
	if ssh {
		cloneOpts.Auth = pcx.GitAuth
	}

	print.Verb("executing clone with options:", cloneOpts)
	repo, err := git.PlainClone(to, false, cloneOpts)
	if err != nil {
		print.Verb("clone failed, cleaning up:", err)
		os.RemoveAll(to)
		return nil, errors.Wrap(err, "failed to clone repository")
	}

	valid, validationErr := ValidateRepository(to)
	if validationErr != nil || !valid {
		print.Verb("cloned repository failed validation")
		os.RemoveAll(to)
		if validationErr != nil {
			return nil, errors.Wrap(validationErr, "cloned repository is invalid")
		}
		return nil, errors.New("cloned repository failed validation")
	}

	print.Verb("repository cloned and validated successfully")
	return repo, nil
}

// updateRepository attempts to update an existing repository with robust error handling
func (pcx PackageContext) updateRepository(repo *git.Repository, to, branch string, ssh bool, from string) (*git.Repository, error) {
	print.Verb("updating repository at", to)

	wt, err := repo.Worktree()
	if err != nil {
		// Worktree error often indicates corruption - re-clone
		print.Verb("worktree error, repository may be corrupted:", err)
		return pcx.recoverByReclone(from, to, branch, ssh)
	}

	// Configure pull options
	pullOpts := &git.PullOptions{}
	if branch != "" {
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + branch)
	}
	if ssh {
		pullOpts.Auth = pcx.GitAuth
	}

	print.Verb("pulling latest changes")
	err = wt.Pull(pullOpts)

	if err != nil && err != git.NoErrAlreadyUpToDate {
		print.Verb("pull failed:", err)
		repairErr := RepairRepository(to)
		if repairErr == nil {
			print.Verb("repository repaired, retrying pull")
			err = wt.Pull(pullOpts)
			if err == nil || err == git.NoErrAlreadyUpToDate {
				return repo, nil
			}
		}

		print.Verb("repair unsuccessful, re-cloning repository")
		return pcx.recoverByReclone(from, to, branch, ssh)
	}

	return repo, nil
}

// recoverByReclone removes a repository and clones it fresh
func (pcx PackageContext) recoverByReclone(from, to, branch string, ssh bool) (*git.Repository, error) {
	print.Verb("recovering repository by re-cloning")

	err := os.RemoveAll(to)
	if err != nil {
		return nil, errors.Wrap(err, "failed to remove corrupted repository for recovery")
	}

	return pcx.cloneRepository(from, to, branch, ssh)
}

// ensureRepoExistsWithMeta wraps ensureRepoExists and provides improved error handling
func (pcx PackageContext) ensureRepoExistsWithMeta(
	meta versioning.DependencyMeta,
	from,
	to,
	branch string,
	ssh,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	repo, err = pcx.ensureRepoExists(from, to, branch, ssh, forceUpdate)
	if err != nil {
		err = WrapGitError(err, meta)
	}
	return repo, err
}
