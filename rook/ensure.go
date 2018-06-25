package rook

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// ErrNotRemotePackage describes a repository that does not contain a package definition file
var ErrNotRemotePackage = errors.New("remote repository does not declare a package")

// EnsureDependencies traverses package dependencies and ensures they are up to date
func (pcx *PackageContext) EnsureDependencies(ctx context.Context, forceUpdate bool) (err error) {
	if pcx.Package.LocalPath == "" {
		return errors.New("package does not represent a locally stored package")
	}

	if !util.Exists(pcx.Package.LocalPath) {
		return errors.New("package local path does not exist")
	}

	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")

	for _, dependency := range pcx.AllDependencies {
		errInner := pcx.EnsurePackage(dependency, forceUpdate)
		if errInner != nil {
			print.Warn(errors.Wrapf(errInner, "failed to ensure package %s", dependency))
			return
		}
		print.Info(pcx.Package, "successfully ensured dependency files for", dependency)
	}

	if pcx.Package.Local && pcx.Package.Runtime != nil {
		print.Verb(pcx.Package, "ensuring local runtime dependencies to", pcx.Package.LocalPath)
		pcx.Package.Runtime.WorkingDir = pcx.Package.LocalPath
		pcx.Package.Runtime.Format = pcx.Package.Format
		err = runtime.Ensure(ctx, pcx.GitHub, pcx.Package.Runtime, false)
		if err != nil {
			return
		}
	}

	return
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func (pcx *PackageContext) EnsurePackage(meta versioning.DependencyMeta, forceUpdate bool) (err error) {
	var (
		dependencyPath = filepath.Join(pcx.Package.Vendor, meta.Repo)
		needToClone    = false // do we need to clone a new repo?
		head           *plumbing.Reference
	)

	repo, err := git.PlainOpen(dependencyPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		return errors.Wrap(err, "failed to open dependency repository")
	} else if err == git.ErrRepositoryNotExists {
		print.Verb(meta, "package does not exist at", dependencyPath, "cloning new copy")
		needToClone = true
	} else {
		head, err = repo.Head()
		if err != nil {
			print.Verb(meta, "package already exists but failed to get repository HEAD:", err)
			needToClone = true
			err = os.RemoveAll(dependencyPath)
			if err != nil {
				return errors.Wrap(err, "failed to temporarily remove possibly corrupted dependency repo")
			}
		} else {
			print.Verb(meta, "package already exists at", head)
		}
	}

	if needToClone {
		print.Verb(meta, "need to clone new copy from cache")
		repo, err = pcx.EnsureDependencyFromCache(meta, dependencyPath, false)
		if err != nil {
			return errors.Wrap(err, "failed to ensure dependency in cache")
		}
	}

	print.Verb(meta, "updating dependency package")
	err = pcx.updateRepoState(repo, meta, forceUpdate)
	if err != nil {
		// try once more, but force a pull
		print.Verb(meta, "unable to update repo in given state, force-pulling latest from repo tip")
		err = pcx.updateRepoState(repo, meta, true)
		if err != nil {
			return errors.Wrap(err, "failed to update repo state")
		}
	}

	pkg, err := types.GetCachedPackage(meta, pcx.CacheDir)
	if err != nil {
		return
	}

	var includePath string
	for _, resource := range pkg.Resources {
		if resource.Platform != pcx.Platform || len(resource.Includes) == 0 {
			continue
		}

		includePath, err = pcx.extractResourceDependencies(context.Background(), pkg, resource)
		if err != nil {
			return
		}
		pcx.AllIncludePaths = append(pcx.AllIncludePaths, includePath)
		print.Verb(includePath)
	}

	return
}

func (pcx PackageContext) extractResourceDependencies(ctx context.Context, pkg types.Package, res types.Resource) (dir string, err error) {
	dir = filepath.Join(pcx.Package.Vendor, res.Path(pkg))
	print.Verb(pkg, "installing resource-based dependency", res.Name, "to", dir)

	err = os.MkdirAll(dir, 0700)
	if err != nil {
		err = errors.Wrap(err, "failed to create target directory")
		return
	}

	_, err = runtime.EnsureVersionedPlugin(ctx, pcx.GitHub, pkg.DependencyMeta, dir, pcx.Platform, pcx.CacheDir, false, true, false)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure asset")
		return
	}

	return
}

// updateRepoState takes a repo that exists on disk and ensures it matches tag, branch or commit constraints
func (pcx *PackageContext) updateRepoState(repo *git.Repository, meta versioning.DependencyMeta, forcePull bool) (err error) {
	print.Verb(meta, "updating repository state with", pcx.GitAuth, "authentication method")

	var wt *git.Worktree
	if forcePull {
		print.Verb(meta, "performing forced pull to latest tip")
		repo, err = pcx.EnsureDependencyFromCache(meta, filepath.Join(pcx.Package.Vendor, meta.Repo), true)
		if err != nil {
			return errors.Wrap(err, "failed to ensure dependency in cache")
		}
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}

		err = wt.Pull(&git.PullOptions{
			Depth: 1000, // get full history
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to force pull for full update")
		}
	} else {
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}
	}

	var (
		ref      *plumbing.Reference
		pullOpts = &git.PullOptions{}
	)

	if meta.SSH != "" {
		pullOpts.Auth = pcx.GitAuth
	}

	if meta.Tag != "" {
		print.Verb(meta, "package has tag constraint:", meta.Tag)

		ref, err = RefFromTag(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from tag")
		}
	} else if meta.Branch != "" {
		print.Verb(meta, "package has branch constraint:", meta.Branch)

		pullOpts.Depth = 1000 // get full history
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + meta.Branch)

		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo branch")
		}

		ref, err = RefFromBranch(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from branch")
		}
	} else if meta.Commit != "" {
		pullOpts.Depth = 1000 // get full history

		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo")
		}

		ref, err = RefFromCommit(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from commit")
		}
	}

	if ref != nil {
		print.Verb(meta, "checking out ref determined from constraint:", ref)

		err = wt.Checkout(&git.CheckoutOptions{
			Hash:  ref.Hash(),
			Force: true,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
		}
		print.Verb(meta, "successfully checked out to", ref.Hash())
	} else {
		print.Verb(meta, "package does not have version constraint pulling latest")

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
func RefFromCommit(repo *git.Repository, meta versioning.DependencyMeta) (ref *plumbing.Reference, err error) {
	commits, err := repo.CommitObjects()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo commits")
		return
	}
	defer commits.Close()

	err = commits.ForEach(func(commit *object.Commit) error {
		hash := commit.Hash.String()

		print.Verb(meta, "checking commit", hash, "<>", meta.Commit)
		if hash == meta.Commit {
			print.Verb(meta, "match found")
			ref = plumbing.NewHashReference(plumbing.ReferenceName(hash), commit.Hash)
			return storer.ErrStop
		}

		return nil
	})
	if err != nil {
		err = errors.Wrap(err, "failed to iterate commits")
	}
	if ref == nil {
		err = errors.Errorf("no commit named '%s' found", meta.Commit)
	}
	return
}
