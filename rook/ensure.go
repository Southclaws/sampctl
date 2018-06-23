package rook

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// ErrNotRemotePackage describes a repository that does not contain a package definition file
var ErrNotRemotePackage = errors.New("remote repository does not declare a package")

// EnsureDependencies traverses package dependencies and ensures they are up to date
func EnsureDependencies(ctx context.Context, gh *github.Client, pkg *types.Package, auth transport.AuthMethod, platform, cacheDir string) (err error) {
	if pkg.LocalPath == "" {
		return errors.New("package does not represent a locally stored package")
	}

	if !util.Exists(pkg.LocalPath) {
		return errors.New("package local path does not exist")
	}

	pkg.Vendor = filepath.Join(pkg.LocalPath, "dependencies")

	visited := make(map[string]bool)
	visited[pkg.DependencyMeta.Repo] = true

	// TODO: remove this recursion, EnsureDependenciesCached does this job already
	var recurse func(meta versioning.DependencyMeta)
	recurse = func(meta versioning.DependencyMeta) {
		pkgPath := filepath.Join(pkg.Vendor, meta.Repo)

		errInner := EnsurePackage(pkgPath, meta, auth, false)
		if errInner != nil {
			print.Warn(errors.Wrapf(errInner, "failed to ensure package %s", meta))
			return
		}

		print.Info(pkg, "successfully ensured dependency files for", meta)

		pkg.AllDependencies = append(pkg.AllDependencies, meta)
		visited[meta.Repo] = true

		var subPkg types.Package
		subPkg, errInner = types.PackageFromDir(pkgPath)
		if errInner != nil {
			print.Warn(pkg, meta, errInner)
			return
		}
		subPkg.DependencyMeta = meta

		var resIncs []string
		for _, res := range subPkg.Resources {
			if res.Archive && res.Platform == platform {
				dir := filepath.Join(pkg.Vendor, res.Path(subPkg))

				print.Verb(subPkg, "installing resource-based dependency", res.Name, "to", dir)

				errInner = os.MkdirAll(dir, 0700)
				if errInner != nil {
					print.Warn(subPkg, "failed to create asset directory:", errInner)
					continue
				}

				_, errInner = runtime.EnsureVersionedPlugin(ctx, gh, subPkg.DependencyMeta, dir, platform, cacheDir, false, true, false)
				if errInner != nil {
					print.Warn(subPkg, "failed to ensure asset:", errInner)
					continue
				}
			}
		}
		pkg.AllIncludePaths = append(pkg.AllIncludePaths, resIncs...)

		var subPkgDepMeta versioning.DependencyMeta
		for _, subPkgDep := range subPkg.Dependencies {
			subPkgDepMeta, errInner = subPkgDep.Explode()
			if errInner != nil {
				continue
			}
			if _, ok := visited[subPkgDepMeta.Repo]; !ok {
				recurse(subPkgDepMeta)
			}
		}
	}

	var meta versioning.DependencyMeta
	for _, dep := range pkg.GetAllDependencies() {
		meta, err = dep.Explode()
		if err != nil {
			return
		}
		recurse(meta)
	}

	if pkg.Local && pkg.Runtime != nil {
		print.Verb(pkg, "ensuring local runtime dependencies to", pkg.LocalPath)
		pkg.Runtime.WorkingDir = pkg.LocalPath
		pkg.Runtime.Format = pkg.Format
		err = runtime.Ensure(ctx, gh, pkg.Runtime, false)
		if err != nil {
			return
		}
	}

	return
}

// func checkConflicts(dependencies []versioning.DependencyMeta) (result []versioning.DependencyMeta) {
// 	exists := make(map[versioning.DependencyMeta]bool)
// 	for _, depMeta := range dependencies {
// 		if !exists[depMeta] {
// 			exists[depMeta] = true
// 			result = append(result, depMeta)
// 		}
// 	}
// 	return
// }

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func EnsurePackage(pkgPath string, meta versioning.DependencyMeta, auth transport.AuthMethod, forceUpdate bool) (err error) {
	var (
		needToClone = false // do we need to clone a new repo?
		head        *plumbing.Reference
	)

	repo, err := git.PlainOpen(pkgPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		return errors.Wrap(err, "failed to open dependency repository")
	} else if err == git.ErrRepositoryNotExists {
		print.Verb(meta, "package does not exist at", util.RelPath(pkgPath), "cloning new copy")
		needToClone = true
	} else {
		head, err = repo.Head()
		if err != nil {
			print.Verb(meta, "package already exists but failed to get repository HEAD:", err)
			needToClone = true
			err = os.RemoveAll(pkgPath)
			if err != nil {
				return errors.Wrap(err, "failed to temporarily remove possibly corrupted dependency repo")
			}
		} else {
			print.Verb(meta, "package already exists at", head)
		}
	}

	if needToClone {
		repo, err = CloneDependency(meta, pkgPath, auth)
		if err != nil {
			return errors.Wrap(err, "failed to clone dependency")
		}
	}

	if forceUpdate {
		print.Verb(meta, "updating dependency package")
		err = updateRepoState(repo, meta, auth, false)
		if err != nil {
			// try once more, but force a pull
			print.Verb(meta, "unable to update repo in given state, force-pulling latest from repo tip")
			err = updateRepoState(repo, meta, auth, true)
			if err != nil {
				return errors.Wrap(err, "failed to update repo state")
			}
		}
	}

	head, err = repo.Head()
	if err != nil {
		return errors.Wrap(err, "failed to check repo HEAD after update")
	}
	print.Verb(meta, "successfully checked out to", head.Hash().String())

	return
}

// updateRepoState takes a repo that exists on disk and ensures it matches tag, branch or commit constraints
func updateRepoState(repo *git.Repository, meta versioning.DependencyMeta, auth transport.AuthMethod, forcePull bool) (err error) {
	var wt *git.Worktree
	wt, err = repo.Worktree()
	if err != nil {
		return errors.Wrap(err, "failed to get repo worktree")
	}

	print.Verb(meta, "updating repository state with", auth, "authentication method")

	if forcePull {
		err = wt.Pull(&git.PullOptions{
			Depth: 1000, // get full history
		})
		if err != nil {
			return errors.Wrap(err, "failed to force pull for full update")
		}
	}

	var (
		ref  *plumbing.Reference
		hash plumbing.Hash
	)
	if meta.Tag != "" {
		print.Verb(meta, "package has tag constraint:", meta.Tag)

		ref, err = RefFromTag(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from tag")
		}
		hash = ref.Hash()
	} else if meta.Branch != "" {
		print.Verb(meta, "package has branch constraint:", meta.Branch)

		pullOpts := &git.PullOptions{
			Depth:         1000, // get full history
			ReferenceName: plumbing.ReferenceName("refs/heads/" + meta.Branch),
		}

		if meta.SSH != "" {
			pullOpts.Auth = auth
		}

		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo branch")
		}

		ref, err = RefFromBranch(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from branch")
		}
		hash = ref.Hash()
	} else if meta.Commit != "" {
		pullOpts := &git.PullOptions{
			Depth: 1000, // get full history
		}

		if meta.SSH != "" {
			pullOpts.Auth = auth
		}

		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo")
		}

		hash, err = RefFromCommit(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from commit")
		}
	}

	if ref != nil {
		print.Verb(meta, "checking out ref determined from constraint:", ref)

		err = wt.Checkout(&git.CheckoutOptions{
			Hash:  hash,
			Force: true,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
		}
	} else {
		print.Verb(meta, "package does not have version constraint pulling latest")

		pullOpts := &git.PullOptions{}

		if meta.SSH != "" {
			pullOpts.Auth = auth
		}

		err = wt.Pull(pullOpts)
		if err != nil {
			if err == git.NoErrAlreadyUpToDate {
				err = nil
			} else {
				return errors.Wrap(err, "failed to fetch latest package")
			}
		}
	}

	return
}

// RefFromTag returns a ref from a given tag
func RefFromTag(repo *git.Repository, meta versioning.DependencyMeta) (ref *plumbing.Reference, err error) {
	constraint, constraintErr := semver.NewConstraint(meta.Tag)
	versionedTags, err := versioning.GetRepoSemverTags(repo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repo tags")
	}

	if constraintErr != nil || len(versionedTags) == 0 {
		print.Verb(meta, "specified version or repo tags not semantic versions", constraintErr)

		var tags storer.ReferenceIter
		tags, err = repo.Tags()
		if err != nil {
			err = errors.Wrap(err, "failed to get repo tags")
			return nil, err
		}
		defer tags.Close()

		tagList := []string{}
		err = tags.ForEach(func(pr *plumbing.Reference) error {
			tag := pr.Name().Short()
			if tag == meta.Tag {
				ref = pr
				return storer.ErrStop
			}
			tagList = append(tagList, tag)
			return nil
		})
		if err != nil {
			err = errors.Wrap(err, "failed to iterate tags")
		}

		if ref == nil {
			err = errors.Errorf("failed to satisfy constraint, '%s' not in %v", meta.Tag, tagList)
		}
	} else {
		print.Verb(meta, "specified version and repo tags are semantic versions")

		sort.Sort(sort.Reverse(versionedTags))

		for _, version := range versionedTags {
			if !constraint.Check(version.Version) {
				print.Verb(meta, "incompatible tag", version.Name, "does not satisfy constraint", meta.Tag)
				continue
			}

			print.Verb(meta, "discovered tag", version.Version, "that matches constraint", meta.Tag)
			ref = version.Ref
			break
		}

		if ref == nil {
			err = errors.Errorf("failed to satisfy constraint, '%s' not in %v", meta.Tag, versionedTags)
		}
	}

	return
}

// RefFromBranch returns a ref from a branch name
func RefFromBranch(repo *git.Repository, meta versioning.DependencyMeta) (ref *plumbing.Reference, err error) {
	branches, err := repo.Branches()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo branches")
		return nil, err
	}
	defer branches.Close()

	branchList := []string{}
	err = branches.ForEach(func(pr *plumbing.Reference) error {
		branch := pr.Name().Short()

		print.Verb(meta, "checking branch", branch)
		if branch == meta.Branch {
			ref = pr
			return storer.ErrStop
		}
		branchList = append(branchList, branch)

		return nil
	})
	if err != nil {
		err = errors.Wrap(err, "failed to iterate branches")
	}
	if ref == nil {
		err = errors.Errorf("no branch named '%s' found in %v", meta.Branch, branchList)
	}
	return
}

// RefFromCommit returns a ref from a commit hash
func RefFromCommit(repo *git.Repository, meta versioning.DependencyMeta) (result plumbing.Hash, err error) {
	commits, err := repo.CommitObjects()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo commits")
		return
	}
	defer commits.Close()

	err = commits.ForEach(func(commit *object.Commit) error {
		hash := commit.Hash.String()

		print.Verb(meta, "checking commit", hash)
		if hash == meta.Commit {
			result = commit.Hash
			return storer.ErrStop
		}

		return nil
	})
	if err != nil {
		err = errors.Wrap(err, "failed to iterate commits")
	}
	if result.IsZero() {
		err = errors.Errorf("no commit named '%s' found", meta.Commit)
	}
	return
}
