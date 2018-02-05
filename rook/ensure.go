package rook

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"github.com/src-d/go-git/plumbing/storer"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// ensure.go contains functions to install, update and validate dependencies of a project.

// ErrNotRemotePackage describes a repository that does not contain a package definition file
var ErrNotRemotePackage = errors.New("remote repository does not declare a package")

// VersionedTag represents a git tag ref with a valid semantic version number as a tag
type VersionedTag struct {
	Ref *plumbing.Reference
	Tag *semver.Version
}

// VersionedTags is just for implementing the Sort interface
type VersionedTags []VersionedTag

// EnsureDependencies traverses package dependencies and ensures they are up to date
func EnsureDependencies(pkg *types.Package) (err error) {
	if pkg.Local == "" {
		return errors.New("package does not represent a locally stored package")
	}

	if !util.Exists(pkg.Local) {
		return errors.New("package local path does not exist")
	}

	pkg.Vendor = filepath.Join(pkg.Local, "dependencies")

	visited := make(map[string]bool)
	visited[pkg.DependencyMeta.Repo] = true

	var recurse func(meta versioning.DependencyMeta)
	recurse = func(meta versioning.DependencyMeta) {
		pkgPath := filepath.Join(pkg.Vendor, meta.Repo)

		err = EnsurePackage(pkgPath, meta)
		if err != nil {
			print.Warn(errors.Wrapf(err, "failed to ensure package %s", meta))
			return
		}

		print.Info(pkg, "successfully ensured dependency files for", meta)

		pkg.AllDependencies = append(pkg.AllDependencies, meta)
		visited[meta.Repo] = true

		subPkg, err := PackageFromDir(false, pkgPath, pkg.Vendor)
		if err != nil {
			print.Warn(pkg, meta, err)
			return
		}

		for _, subPkgDep := range subPkg.Dependencies {
			subPkgDepMeta, err := subPkgDep.Explode()
			if err != nil {
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

	return
}

func checkConflicts(dependencies []versioning.DependencyMeta) (result []versioning.DependencyMeta) {
	exists := make(map[versioning.DependencyMeta]bool)
	for _, depMeta := range dependencies {
		if !exists[depMeta] {
			exists[depMeta] = true
			result = append(result, depMeta)
		}
	}
	return
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func EnsurePackage(pkgPath string, meta versioning.DependencyMeta) (err error) {
	repo, err := git.PlainOpen(pkgPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		err = errors.Wrap(err, "failed to open dependency repository")
		return
	}

	// determine if we need to clone the repository first
	needToClone := false
	if err == git.ErrRepositoryNotExists {
		print.Verb(meta, "package does not exist at", pkgPath, "cloning new copy")
		needToClone = true
	} else {
		head, err := repo.Head()
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
		print.Verb(meta, "cloning dependency package:", meta)
		repo, err = git.PlainClone(pkgPath, false, &git.CloneOptions{
			URL:   meta.URL(),
			Depth: 1,
		})
		if err != nil {
			err = errors.Wrap(err, "failed to clone dependency repository")
			return
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo worktree")
		return
	}

	// determine if we need to check out a specific
	var ref *plumbing.Reference
	if meta.Tag != "" {
		print.Verb(meta, "package has tag constraint:", meta.Tag)

		ref, err = RefFromTag(repo, meta, meta.Tag)
		if err != nil {
			return
		}
	} else if meta.Branch != "" {
		// todo
	} else if meta.Commit != "" {
		// todo
	} else {
		print.Verb(meta, "package does not have version constraint, using latest")

		err = wt.Pull(&git.PullOptions{})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			err = errors.Wrap(err, "failed to fetch latest package")
			return
		}
	}

	if ref != nil {
		err = wt.Checkout(&git.CheckoutOptions{
			Hash:  ref.Hash(),
			Force: true,
		})
		if err != nil {
			err = errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
			return
		}
	}

	head, err := repo.Head()
	if err != nil {
		return
	}
	print.Verb(meta, "successfully checked out to", head.Hash().String())

	return
}

// RefFromTag returns a ref from a given tag
func RefFromTag(repo *git.Repository, meta versioning.DependencyMeta, version string) (ref *plumbing.Reference, err error) {
	constraint, err := semver.NewConstraint(version)
	if err != nil {
		err = nil

		tags, err := repo.Tags()
		if err != nil {
			err = errors.Wrap(err, "failed to get repo tags")
			return nil, err
		}
		defer tags.Close()

		tags.ForEach(func(pr *plumbing.Reference) error {
			tag := pr.Name().Short()

			if tag == meta.Tag {
				ref = pr
				return storer.ErrStop
			}

			return nil
		})

	} else {
		var versionedTags VersionedTags
		versionedTags, err = GetRepoSemverTags(repo)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get repo tags")
		}

		sort.Sort(sort.Reverse(versionedTags))

		for _, version := range versionedTags {
			if constraint.Check(version.Tag) {
				print.Verb(meta, "discovered tag", version.Tag, "that matches constraint", meta.Tag)
				ref = version.Ref
				return
			}

			// these messages will be removed in future versions
			print.Verb(meta, "incompatible tag", version.Tag, "does not satisfy constraint", meta.Tag)
		}
		err = errors.Errorf("failed to satisfy constraint, no tag found by that name, available tags: %v", versionedTags)
	}

	return
}

// Implements the sort interface on collections of VersionedTags - code copied from semver because
// VersionedTags is just a copy of semver.Collection with the added git ref field

// Len returns the length of a collection. The number of Version instances
// on the slice.
func (c VersionedTags) Len() int {
	return len(c)
}

// Less is needed for the sort interface to compare two Version objects on the
// slice. If checks if one is less than the other.
func (c VersionedTags) Less(i, j int) bool {
	return c[i].Tag.LessThan(c[j].Tag)
}

// Swap is needed for the sort interface to replace the Version objects
// at two different positions in the slice.
func (c VersionedTags) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// GetRepoSemverTags returns a list of tags that are valid semantic versions
func GetRepoSemverTags(repo *git.Repository) (versionedTags VersionedTags, err error) {
	tags, err := repo.Tags()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo tags")
		return
	}
	defer tags.Close()

	tags.ForEach(func(pr *plumbing.Reference) error {
		tag := pr.Name().Short()

		tagVersion, err := semver.NewVersion(tag)
		if err != nil {
			return nil
		}

		versionedTags = append(versionedTags, VersionedTag{
			Ref: pr,
			Tag: tagVersion,
		})

		return nil
	})

	return
}
