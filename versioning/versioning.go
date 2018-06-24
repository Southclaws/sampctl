package versioning

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
)

// DependencyString represents a git repository via various patterns
type DependencyString string

// DependencyMeta represents all the individual components of a DependencyString
type DependencyMeta struct {
	Site   string `json:"site,omitempty"`                           // The site the repo exists on, default is github.com
	User   string `json:"user"`                                     // Repository owner
	Repo   string `json:"repo"`                                     // Repository name
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`     // Optional subdirectory for .inc files
	Tag    string `json:"tag,omitempty" yaml:"tag,omitempty"`       // Target tag
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"` // Target branch
	Commit string `json:"commit,omitempty" yaml:"commit,omitempty"` // Target commit sha
	SSH    string `json:"ssh,omitempty" yaml:"ssh,omitempty"`       // SSH user (usually 'git')
}

func (dm DependencyMeta) String() string {
	var site string
	if dm.Site != "" {
		site = dm.Site + "/"
	}

	if dm.Tag != "" {
		return fmt.Sprintf("%s%s/%s:%s", site, dm.User, dm.Repo, dm.Tag)
	} else if dm.Branch != "" {
		return fmt.Sprintf("%s%s/%s@%s", site, dm.User, dm.Repo, dm.Branch)
	} else if dm.Commit != "" {
		return fmt.Sprintf("%s%s/%s#%s", site, dm.User, dm.Repo, dm.Commit)
	}
	return fmt.Sprintf("%s%s/%s", site, dm.User, dm.Repo)
}

// CachePath returns the path from the cache to a cached package
func (dm DependencyMeta) CachePath(cacheDir string) (path string) {
	var branch string
	if dm.Branch == "" {
		branch = "default"
	} else {
		branch = dm.Branch
	}
	return filepath.Join(cacheDir, "packages", dm.User, dm.Repo, branch)
}

// Validate checks for errors in a DependencyMeta object
func (dm DependencyMeta) Validate() (err error) {
	if dm.User == "" {
		return errors.New("dependency meta missing user")
	}
	if dm.Repo == "" {
		return errors.New("dependency meta missing repo")
	}
	return
}

var (
	// MatchGitSSH matches ssh URLs such as 'git@github.com:Southclaws/sampctl'
	MatchGitSSH = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]+)\@((?:[a-zA-Z][a-zA-Z0-9\-]*\.)*[a-zA-Z][a-zA-Z0-9\-]*)\:((?:[A-Za-z0-9_\-\.]+\/?)*)$`)
	// MatchDependencyString matches a dependency string such as 'Username/Repository:tag', 'Username/Repository@branch', 'Username/Repository#commit'
	MatchDependencyString = regexp.MustCompile(`^\/?([a-zA-Z0-9-]+)\/([a-zA-Z0-9-._]+)(?:\/)?([a-zA-Z0-9-_$\[\]{}().,\/]*)?((?:@)|(?:\:)|(?:#))?(.+)?$`)
)

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
	u, err := url.Parse(string(d))
	if err == nil {

		path := u.Path

		if u.Fragment != "" {
			path += "#" + u.Fragment
		}

		dep, err = explodePath(path)
		dep.Site = u.Host
	} else {
		user, host, path, success := attemptGitSSH(string(d))
		if success {
			dep, err = explodePath(path)
			dep.Site = host
			dep.SSH = user
		} else {
			dep, err = explodePath(string(d))
		}
	}

	// default to github
	if dep.Site == "" {
		dep.Site = "github.com"
	}

	if err == nil {
		err = dep.Validate()
	}

	// if there's an error, return an empty meta object
	if err != nil {
		return DependencyMeta{}, err
	}

	return
}

func attemptGitSSH(d string) (username, host, path string, success bool) {
	if !MatchGitSSH.MatchString(d) {
		return
	}

	captures := MatchGitSSH.FindStringSubmatch(d)
	if len(captures) != 4 {
		return
	}

	return captures[1], captures[2], captures[3], true
}

func explodePath(d string) (dep DependencyMeta, err error) {
	if !MatchDependencyString.MatchString(d) {
		err = errors.New("dependency string does not match pattern")
		return
	}

	captures := MatchDependencyString.FindStringSubmatch(d)
	if len(captures) != 6 {
		err = errors.New("dependency pattern match count != 6")
		return
	}

	dep.User = captures[1]
	dep.Repo = captures[2]
	dep.Path = captures[3]

	if len(captures[4]) == 1 && len(captures[5]) > 0 {
		switch captures[4][0] {
		case ':':
			dep.Tag = captures[5]
		case '@':
			dep.Branch = captures[5]
		case '#':
			if len(captures[5]) != 40 {
				err = errors.Errorf("dependency string specifies a commit hash with an incorrect length (%d)", len(captures[5]))
			}
			dep.Commit = captures[5]
		default:
			err = errors.New("version must be a branch (@) or a tag (:)")
		}
	}
	return
}

// URL generates a GitHub URL for a package - it does not test the validity of the URL
func (dm DependencyMeta) URL() string {
	if dm.SSH != "" {
		return fmt.Sprintf("%s@%s:%s/%s", dm.SSH, dm.Site, dm.User, dm.Repo)
	}

	return fmt.Sprintf("https://%s/%s/%s", dm.Site, dm.User, dm.Repo)
}
