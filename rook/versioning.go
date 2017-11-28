package rook

import (
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
)

// DependencyString represents a GitHub repository via various patterns
type DependencyString string

// Dependency represents all the components required to locate a package version
type Dependency struct {
	User    string `json:"user"`    // Owner of the project repository
	Repo    string `json:"repo"`    // GitHub repository name
	Path    string `json:"path"`    // Subdirectory that contains .inc files (if any)
	Version string `json:"version"` // Version string (git tag, preferably a semantic version)
}

var dependencyPattern = regexp.MustCompile(`^((?:http(?:s)?:\/\/)?github.com\/)?([a-zA-Z0-9-]*)\/([a-zA-Z0-9-_]*)(?:\/)?([a-zA-Z0-9-_$\[\]{}().,\/]*)?(?:\:)?(.*)?$`)

// Validate checks if a dependency is of a valid pattern
// a valid pattern is either a GitHub URL or just a user/repo combination followed by an optional
// versioning string which is either a semantic version number or a SHA1 hash.
//
// Examples of valid dependency strings ignoring versioning:
//   https://github.com/user/repo
//   http://github.com/user/repo
//   github.com/user/repo
//   user/repo
//
// And, examples of valid dependency strings with the user/repo:version example followed by a
// description of what the constraint means.
// (More info: https://github.com/Masterminds/semver#basic-comparisons)
//   user/repo:1.2.3 (force version 1.2.3)
//   user/repo:1.2.x (allow any 1.2 build)
//   user/repo:2.x (allow any version 2 minor)
//
// And finally, examples of dependency strings with additional paths for when include files exist in
// a subdirectory of the repository.
//   https://github.com/user/repo/includes:1.2.3
//   http://github.com/user/repo/includes:1.2.3
//   github.com/user/repo/includes:1.2.3
//   user/repo/includes:1.2.3
//
// Please note that this function may return true AND an error, this means the dependency version is
// valid but does not quite satisfy the criteria for versioning. This is because not all projects
// use either git tagging or semantic versioning.
func (d DependencyString) Validate() (bool, error) {
	if !dependencyPattern.MatchString(string(d)) {
		return false, errors.New("dependency string does not match pattern")
	}

	captures := dependencyPattern.FindStringSubmatch(string(d))
	if len(captures) != 6 {
		return false, errors.New("dependency pattern match count != 6")
	}

	if captures[5] != "" {
		_, err := semver.NewConstraint(captures[5])
		if err != nil {
			return true, errors.Wrap(err, "dependency version does not match semantic versioning pattern")
		}
	}

	return true, nil
}

// Explode does a similar job to Validate - splits the specified dependency string into it's
// component parts and validates it, this function returns the component parts.
func (d DependencyString) Explode() (dep Dependency, err error) {
	if !dependencyPattern.MatchString(string(d)) {
		err = errors.New("dependency string does not match pattern")
		return
	}

	captures := dependencyPattern.FindStringSubmatch(string(d))
	if len(captures) != 6 {
		err = errors.New("dependency pattern match count != 6")
		return
	}

	dep.User = captures[2]
	dep.Repo = captures[3]
	dep.Path = captures[4]
	dep.Version = captures[5]

	return
}
