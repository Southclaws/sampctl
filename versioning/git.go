package versioning

import (
	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"
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
