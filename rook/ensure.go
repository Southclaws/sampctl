package rook

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// ensure.go contains functions to install, update and validate dependencies of a project.

// VersionedTag represents a git tag ref with a valid semantic version number as a tag
type VersionedTag struct {
	Ref *plumbing.Reference
	Tag *semver.Version
}

// VersionedTags is just for implementing the Sort interface
type VersionedTags []VersionedTag

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func EnsurePackage(vendorDirectory string, pkg Package) (err error) {
	pkgPath := filepath.Join(util.FullPath(vendorDirectory), pkg.Repo)

	repo, err := git.PlainOpen(pkgPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		err = errors.Wrap(err, "failed to open dependency repository")
		return
	}

	if err == git.ErrRepositoryNotExists {
		fmt.Println(pkg, "package does not exist at", pkgPath, "cloning new copy")

		repo, err = git.PlainClone(pkgPath, false, &git.CloneOptions{
			URL: pkg.GetURL(),
		})
		if err != nil {
			err = errors.Wrap(err, "failed to clone dependency repository")
			return
		}
	} else {
		head, _ := repo.Head()
		fmt.Println(pkg, "package already exists at", head)
	}

	if pkg.Version == "" {
		err = repo.Fetch(&git.FetchOptions{})
		if err == git.NoErrAlreadyUpToDate {
			fmt.Println(pkg, "package does not have version constraint and the latest copy is already present")
			return nil
		} else if err != nil {
			err = errors.Wrap(err, "failed to fetch latest package")
			return
		} else {
			fmt.Println(pkg, "package does not have version constraint and the latest copy has been cloned")
			return
		}
	}

	versionedTags, err := getPackageRepoTags(repo)
	if err != nil {
		return errors.Wrap(err, "failed to get package repository tags")
	}

	ref, err := getRefFromConstraint(pkg, versionedTags, pkg.Version)
	if err != nil {
		return
	}

	wt, err := repo.Worktree()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo worktree")
		return
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Hash:  ref.Hash(),
		Force: true,
	})
	if err != nil {
		err = errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
		return
	}

	head, err := repo.Head()
	if err != nil {
		return
	}
	fmt.Println(pkg, "successfully checked out to", head.Hash().String())

	return
}

func getPackageRepoTags(repo *git.Repository) (versionedTags VersionedTags, err error) {
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

func getRefFromConstraint(pkg Package, versionedTags VersionedTags, version string) (ref *plumbing.Reference, err error) {
	constraint, err := semver.NewConstraint(version)
	if err != nil {
		// todo: support non-semver versioning by just using tag
		err = errors.Wrap(err, "package version constraint is not valid")
		return
	}

	sort.Sort(sort.Reverse(versionedTags))

	for _, version := range versionedTags {
		if constraint.Check(version.Tag) {
			fmt.Println(pkg, "discovered tag", version.Tag, "that matches constraint", pkg.Version)
			ref = version.Ref
			return
		}

		// these messages will be removed in future versions
		fmt.Println(pkg, "incompatible tag", version.Tag, "does not satisfy constraint", pkg.Version)
	}
	err = errors.Errorf("failed to satisfy constraint, no tag found by that name, available tags: %v", versionedTags)
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
