package rook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

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
func (pkg Package) EnsureDependencies() (allDependencies []versioning.DependencyString, err error) {
	if pkg.local == "" {
		return nil, errors.New("package does not represent a locally stored package")
	}

	if !util.Exists(pkg.local) {
		return nil, errors.New("package local path does not exist")
	}

	pkg.vendor = filepath.Join(pkg.local, "dependencies")

	allDependencies, err = pkg.gather()
	if err != nil {
		return nil, errors.Wrap(err, "failed to gather dependency tree")
	}

	for _, depString := range allDependencies {
		dep, err := PackageFromDep(depString)
		if err != nil {
			return nil, errors.Errorf("package dependency '%s' is invalid: %v", depString, err)
		}

		err = EnsurePackage(pkg.vendor, dep)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to ensure package %s", dep)
		}
	}

	return
}

// gather recursively discovers `pawn.json`/`pawn.yaml` files in dependencies to build up a list of
// packages to ensure.
func (pkg Package) gather() (dependencies []versioning.DependencyString, err error) {
	client := github.NewClient(nil)

	var recurse func(Package)

	recurse = func(innerPkg Package) {
		fmt.Println(innerPkg, "gathering dependencies...")
		for _, depString := range innerPkg.Dependencies {
			depMeta, err := depString.Explode()
			if err != nil {
				fmt.Println(innerPkg, "failed to parse dependency string", depString)
				continue
			}

			fmt.Println(innerPkg, "- gathered dependency", depMeta)

			dependencies = append(dependencies, depString)

			dependencyPkg, err := getRemotePackage(client, depMeta.User, depMeta.Repo)
			if err != nil {
				if err == ErrNotRemotePackage {
					fmt.Println(innerPkg, "dependency", depMeta.Repo, "does not contain package definition")
					continue
				}
				fmt.Println(depMeta, "failed to get remote package manifest:", err)
				continue
			}
			fmt.Println(innerPkg, "- got dependency: ", dependencyPkg.User, dependencyPkg.Repo, dependencyPkg.Version, "%")

			recurse(dependencyPkg)
		}
	}
	recurse(pkg)

	fmt.Println(dependencies)

	dependencies = checkConflicts(dependencies)

	return
}

func getRemotePackage(client *github.Client, user, repo string) (pkg Package, err error) {
	var (
		reader   io.Reader
		contents []byte
	)

	reader, err = client.Repositories.DownloadContents(context.Background(), user, repo, "pawn.json", &github.RepositoryContentGetOptions{})
	if err == nil {
		contents, err = ioutil.ReadAll(reader)
		if err != nil {
			return
		}

		err = json.Unmarshal(contents, &pkg)
		if err != nil {
			return
		}

		pkg.User = user
		pkg.Repo = repo

		return
	}

	reader, err = client.Repositories.DownloadContents(context.Background(), pkg.User, pkg.Repo, "pawn.yaml", &github.RepositoryContentGetOptions{})
	if err == nil {
		contents, err = ioutil.ReadAll(reader)
		if err != nil {
			return
		}

		err = json.Unmarshal(contents, &pkg)
		if err != nil {
			return
		}

		pkg.User = user
		pkg.Repo = repo

		return
	}
	if err == nil {
		err = ErrNotRemotePackage
	}

	return
}

func checkConflicts(dependencies []versioning.DependencyString) (result []versioning.DependencyString) {
	exists := make(map[versioning.DependencyString]bool)
	for _, depString := range dependencies {
		if !exists[depString] {
			exists[depString] = true
			result = append(result, depString)
		}
	}
	return
}

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

	var (
		ref           *plumbing.Reference
		needToClone   bool
		versionedTags VersionedTags
	)

	if err == git.ErrRepositoryNotExists {
		fmt.Println(pkg, "package does not exist at", pkgPath, "cloning new copy")
		needToClone = true
	} else {
		ref, err = repo.Head()
		if err != nil {
			fmt.Println(pkg, "package already exists but failed to get repository HEAD:", err)
			needToClone = true
			err = os.RemoveAll(pkgPath)
			if err != nil {
				return errors.Wrap(err, "failed to temporarily remove possibly corrupted dependency repo")
			}
		} else {
			fmt.Println(pkg, "package already exists at", ref)
		}
	}

	if needToClone {
		fmt.Println(pkg, "cloning dependency package:", pkg)
		repo, err = git.PlainClone(pkgPath, false, &git.CloneOptions{
			URL: pkg.GetURL(),
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

	if pkg.Version == "" {
		fmt.Println(pkg, "package does not have version constraint, fetching latest...")

		err = wt.Pull(&git.PullOptions{})
		if err == git.NoErrAlreadyUpToDate {
			fmt.Println(pkg, "latest copy is already present")
		} else if err != nil {
			err = errors.Wrap(err, "failed to fetch latest package")
			return
		} else {
			fmt.Println(pkg, "latest copy has been fetched")
		}
	} else {
		fmt.Println(pkg, "package has version constraint, checking out...")

		versionedTags, err = getPackageRepoTags(repo)
		if err != nil {
			return errors.Wrap(err, "failed to get package repository tags")
		}

		ref, err = getRefFromConstraint(pkg, versionedTags, pkg.Version)
		if err != nil {
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
