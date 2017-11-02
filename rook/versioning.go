package rook

import (
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
)

// Dependency represents a GitHub repository via various patterns
type Dependency string

var dependencyPattern = regexp.MustCompile(`^((?:http(?:s)?:\/\/)?github.com\/)?([a-zA-Z0-9-]*)\/([a-zA-Z0-9-]*)(?:\:)?(.*)?$`)

// Validate checks if a dependency is of a valid pattern
// a valid pattern is either a GitHub URL or just a user/repo combination followed by an optional
// versioning string which is either a semantic version number or a SHA1 hash.
//
// Examples of valid dependency paths ignoring versioning:
//   https://github.com/user/repo
//   http://github.com/user/repo
//   github.com/user/repo
//   user/repo
//
// And, examples of valid versioning strings with the user/repo example followed by a description
// of what the constraint means, more info: https://github.com/Masterminds/semver#basic-comparisons
//   user/repo:1.2.3 (force version 1.2.3)
//   user/repo:1.2.x (allow any 1.2 build)
//   user/repo:2.x (allow any version 2 minor)
//
// Please note that this function may return true AND an error, this means the dependency version is
// valid but does not quite satisfy the criteria for versioning. This is because not all projects
// use either git tagging or semantic versioning.
func (d Dependency) Validate() (bool, error) {
	if !dependencyPattern.MatchString(string(d)) {
		return false, errors.New("dependency string does not match pattern")
	}

	captures := dependencyPattern.FindStringSubmatch(string(d))
	if len(captures) != 5 {
		return false, errors.New("dependency pattern match count != 5")
	}

	if captures[4] != "" {
		_, err := semver.NewConstraint(captures[4])
		if err != nil {
			return true, errors.Wrap(err, "dependency version does not match semantic versioning pattern")
		}
	}

	return true, nil
}

// Explode does a similar job to Validate - splits the specified dependency string into it's
// component parts and validates it, this function returns the component parts.
func (d Dependency) Explode() (user, repo, version string, err error) {
	if !dependencyPattern.MatchString(string(d)) {
		err = errors.New("dependency string does not match pattern")
		return
	}

	captures := dependencyPattern.FindStringSubmatch(string(d))
	if len(captures) != 5 {
		err = errors.New("dependency pattern match count != 5")
		return
	}

	if captures[1] == "" {
		user = captures[2]
		repo = captures[3]
		version = captures[4]
	} else {
		user = captures[1]
		repo = captures[2]
		version = captures[3]
	}
	return
}
