package versioning

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

// DependencyString represents a git repository via various patterns
type DependencyString string

// DependencyMeta represents all the individual components of a DependencyString
type DependencyMeta struct {
	Site   string `json:"site"`                                     // The site the repo exists on, default is github.com
	User   string `json:"user"`                                     // Repository owner
	Repo   string `json:"repo"`                                     // Repository name
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`     // Optional subdirectory for .inc files
	Tag    string `json:"tag,omitempty" yaml:"tag,omitempty"`       // Target tag
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"` // Target branch
	Commit string `json:"commit,omitempty" yaml:"commit,omitempty"` // Target commit sha
}

func (dm DependencyMeta) String() string {
	if dm.Tag != "" {
		return fmt.Sprintf("%s/%s/%s:%s", dm.Site, dm.User, dm.Repo, dm.Tag)
	} else if dm.Branch != "" {
		return fmt.Sprintf("%s/%s/%s@%s", dm.Site, dm.User, dm.Repo, dm.Branch)
	} else if dm.Commit != "" {
		return fmt.Sprintf("%s/%s/%s#%s", dm.Site, dm.User, dm.Repo, dm.Commit)
	}
	return fmt.Sprintf("%s/%s/%s", dm.Site, dm.User, dm.Repo)
}

var dependencyPattern = regexp.MustCompile(`^((?:[a-z]+://)[a-zA-Z0-9][a-zA-Z0-9-_]{0,61}[a-zA-Z0-9]{0,1}\.(?:[a-zA-Z]{1,6}|[a-zA-Z0-9-]{1,30}\.[a-zA-Z]{2,3})/)?([a-zA-Z0-9-]*)\/([a-zA-Z0-9-._]*)(?:\/)?([a-zA-Z0-9-_$\[\]{}().,\/]*)?((?:@)|(?:\:)|(?:#))?(.+)?$`)

// Explode splits a dependency string into its component parts and returns a meta object
// a valid pattern is either a git URL or just a user/repo combination followed by an optional
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
func (d DependencyString) Explode() (dep DependencyMeta, err error) {
	if !dependencyPattern.MatchString(string(d)) {
		err = errors.New("dependency string does not match pattern")
		return
	}

	captures := dependencyPattern.FindStringSubmatch(string(d))
	if len(captures) != 7 {
		err = errors.New("dependency pattern match count != 7")
		return
	}

	if captures[1] == "" {
		dep.Site = "https://github.com"
	} else {
		dep.Site = strings.TrimRight(captures[1], "/")
	}
	dep.User = captures[2]
	dep.Repo = captures[3]
	dep.Path = captures[4]

	if len(captures[5]) == 1 && len(captures[6]) > 0 {
		switch captures[5][0] {
		case ':':
			dep.Tag = captures[6]
		case '@':
			dep.Branch = captures[6]
		case '#':
			if len(captures[6]) != 40 {
				err = errors.Errorf("dependency string specifies a commit hash with an incorrect length (%d)", len(captures[6]))
			}
			dep.Commit = captures[6]
		default:
			err = errors.New("version must be a branch (@) or a tag (:)")
		}
	}

	return
}

// URL generates a GitHub URL for a package - it does not test the validity of the URL
func (dm DependencyMeta) URL() string {
	return fmt.Sprintf("%s/%s/%s", dm.Site, dm.User, dm.Repo)
}
