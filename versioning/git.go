package versioning

import (
	"sort"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"

	"github.com/Southclaws/sampctl/print"
)

// Implements the sort interface on collections of VersionedTags - code copied from semver because
// VersionedTags is just a copy of semver.Collection with the added git ref field

// VersionedTag represents a git tag ref with a valid semantic version number as a tag
type VersionedTag struct {
	Ref     *plumbing.Reference
	Name    string
	Version *semver.Version
}

// VersionedTags is just for implementing the Sort interface
type VersionedTags []VersionedTag

// Len returns the length of a collection. The number of Version instances
// on the slice.
func (c VersionedTags) Len() int {
	return len(c)
}

// Less is needed for the sort interface to compare two Version objects on the
// slice. If checks if one is less than the other.
func (c VersionedTags) Less(i, j int) bool {
	return c[i].Version.LessThan(c[j].Version)
}

// Swap is needed for the sort interface to replace the Version objects
// at two different positions in the slice.
func (c VersionedTags) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// RefFromTag returns a ref from a given tag
func RefFromTag(repo *git.Repository, meta DependencyMeta) (ref *plumbing.Reference, err error) {
	constraint, constraintErr := semver.NewConstraint(meta.Tag)
	versionedTags, err := GetRepoSemverTags(repo)
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
			refInner, errInner := RefFromTagRef(repo, pr)
			if errInner != nil {
				return nil
			}

			tag := refInner.Name().Short()
			if tag == meta.Tag {
				ref = refInner
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
func RefFromBranch(repo *git.Repository, meta DependencyMeta) (ref *plumbing.Reference, err error) {
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
func RefFromCommit(repo *git.Repository, meta DependencyMeta) (ref *plumbing.Reference, err error) {
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

// GetRepoSemverTags returns a list of tags that are valid semantic versions
func GetRepoSemverTags(repo *git.Repository) (versionedTags VersionedTags, err error) {
	tags, err := repo.Tags()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo tags")
		return
	}
	defer tags.Close()

	err = tags.ForEach(func(pr *plumbing.Reference) error {
		tagName := pr.Name().Short()

		versionNumber, errInner := semver.NewVersion(tagName)
		if errInner != nil {
			return nil
		}

		ref := pr

		if pr.Name().IsTag() {
			ref, errInner = func() (ref *plumbing.Reference, errInnerInner error) {
				refTagObject, errInnerInner := repo.TagObject(pr.Hash())
				if errInnerInner != nil {
					return pr, nil
				}
				refCommit, errInnerInner := refTagObject.Commit()
				if errInnerInner != nil {
					return nil, errInnerInner
				}
				return plumbing.NewHashReference(pr.Name(), refCommit.Hash), nil
			}()
			if errInner != nil {
				return errInner
			}
		}

		versionedTags = append(versionedTags, VersionedTag{
			Ref:     ref,
			Name:    tagName,
			Version: versionNumber,
		})

		return nil
	})
	if err != nil {
		err = errors.Wrap(err, "failed to iterate commits")
	}

	return
}

// GetRepoCurrentVersionedTag returns the current versioned tag of a repo if
// there is one. Otherwise it returns nil.
func GetRepoCurrentVersionedTag(repo *git.Repository) (tag *VersionedTag, err error) {
	head, err := repo.Head()
	if err != nil {
		return
	}

	tags, err := repo.Tags()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo tags")
		return
	}
	defer tags.Close()

	err = tags.ForEach(func(pr *plumbing.Reference) (errInner error) {
		tagName := pr.Name().Short()

		ref, errInner := RefFromTagRef(repo, pr)
		if errInner != nil {
			return
		}

		if ref.Hash() != head.Hash() {
			return
		}

		tag = &VersionedTag{
			Ref:  ref,
			Name: tagName,
		}

		versionNumber, errInner := semver.NewVersion(tagName)
		if errInner == nil {
			tag.Version = versionNumber
		}

		return storer.ErrStop
	})

	return
}

// RefFromTagRef resolves a tag reference to its actual object
func RefFromTagRef(repo *git.Repository, pr *plumbing.Reference) (ref *plumbing.Reference, err error) {
	if !pr.Name().IsTag() {
		return pr, nil
	}

	obj, err := repo.TagObject(pr.Hash())
	if err != nil {
		return pr, nil
	}

	commit, err := obj.Commit()
	if err != nil {
		return
	}

	return plumbing.NewHashReference(pr.Name(), commit.Hash), nil
}
