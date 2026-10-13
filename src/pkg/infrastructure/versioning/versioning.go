package versioning

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

// loadedOverrides caches the dependency overrides loaded from configuration
var loadedOverrides map[string]string

var overridesMu sync.Mutex

// DependencyString represents a git repository via various patterns
type DependencyString string

// DependencyMeta represents all the individual components of a DependencyString
type DependencyMeta struct {
	Site   string `json:"site,omitempty" yaml:"site,omitempty"`     // The site the repo exists on, default is github.com
	User   string `json:"user"`                                     // Repository owner
	Repo   string `json:"repo"`                                     // Repository name
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`     // Optional subdirectory for .inc files
	Tag    string `json:"tag,omitempty" yaml:"tag,omitempty"`       // Target tag
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"` // Target branch
	Commit string `json:"commit,omitempty" yaml:"commit,omitempty"` // Target commit sha
	SSH    string `json:"ssh,omitempty" yaml:"ssh,omitempty"`       // SSH user (usually 'git')

	// URL-like dependency fields
	Scheme string `json:"scheme,omitempty" yaml:"scheme,omitempty"`    // URL scheme (plugin://, includes://, filterscript://, component://)
	Local  string `json:"local,omitempty" yaml:"local_path,omitempty"` // Local path for local schemes
}

func (dm DependencyMeta) String() string {
	if dm.Scheme != "" {
		if dm.Local != "" {
			// Local scheme: scheme://local/path
			return fmt.Sprintf("%s://local/%s", dm.Scheme, dm.Local)
		}
		// Remote scheme: scheme://user/repo[:tag]
		if dm.User != "" && dm.Repo != "" {
			result := fmt.Sprintf("%s://%s/%s", dm.Scheme, dm.User, dm.Repo)
			if dm.Tag != "" {
				result += ":" + dm.Tag
			} else if dm.Branch != "" {
				result += "@" + dm.Branch
			} else if dm.Commit != "" {
				result += "#" + dm.Commit
			}
			return result
		}
	}

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
	// Handle URL-like schemes
	if dm.Scheme != "" {
		switch dm.Scheme {
		case "plugin", "includes", "filterscript", "component":
			if dm.Local != "" {
				// Local schemes only need Local path
				if dm.Local == "" {
					return errors.New("local dependency path is required")
				}
				return nil
			}
			// Remote schemes need User and Repo
			if dm.User == "" {
				return errors.New("dependency repository owner is required")
			}
			if dm.Repo == "" {
				return errors.New("dependency repository name is required")
			}
			return nil
		default:
			return errors.Errorf("unsupported dependency scheme: %s", dm.Scheme)
		}
	}

	if dm.User == "" {
		return errors.New("dependency repository owner is required")
	}
	if dm.Repo == "" {
		return errors.New("dependency repository name is required")
	}
	return
}

// DependencyOverrides maps original dependency strings to replacement dependency strings
// This allows handling cases where repositories have been deleted/moved
var DependencyOverrides = map[string]string{
	"Southclaws/samp-stdlib":                  "pawn-lang/samp-stdlib",
	"sampctl/samp-stdlib":                     "pawn-lang/samp-stdlib",
	"github.com/Zeex/samp-plugin-crashdetect": "github.com/AmyrAhmady/samp-plugin-crashdetect",
	"Zeex/samp-plugin-crashdetect":            "AmyrAhmady/samp-plugin-crashdetect",
}

// ApplyDependencyOverrides checks if a dependency string should be replaced with an override
// and returns the override if one exists, otherwise returns the original string
func ApplyDependencyOverrides(depStr string) string {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	if loadedOverrides == nil {
		loadedOverrides = LoadDependencyOverrides("")
	}
	return applyDependencyOverrides(depStr, loadedOverrides)
}

func applyDependencyOverrides(depStr string, overrides map[string]string) string {
	overrideHasExplicitVersion := func(s string) bool {
		if meta, err := dependencyOverrideMeta(s); err == nil {
			return meta.IsLocalScheme() || meta.Tag != "" || meta.Branch != "" || meta.Commit != ""
		}
		trimmed := strings.TrimPrefix(s, "https://")
		trimmed = strings.TrimPrefix(trimmed, "http://")

		if idx := strings.LastIndex(trimmed, "/"); idx != -1 {
			return strings.ContainsAny(trimmed[idx+1:], ":@#")
		}
		return strings.ContainsAny(trimmed, ":@#")
	}

	// Normalize the dependency string by removing common prefixes
	normalized := strings.TrimPrefix(depStr, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")

	// Check for exact match first
	if override, exists := overrides[depStr]; exists {
		return override
	}

	// Check for normalized match (without protocol)
	if override, exists := overrides[normalized]; exists {
		return override
	}

	var baseDep, versionPart string
	if idx := strings.LastIndex(normalized, "/"); idx != -1 {
		// Only consider version separators after the final path segment.
		if subIdx := strings.IndexAny(normalized[idx+1:], ":@#"); subIdx != -1 {
			idxFull := idx + 1 + subIdx
			baseDep = normalized[:idxFull]
			versionPart = normalized[idxFull:]
		}
	} else {
		// Fallback for unexpected formats.
		if subIdx := strings.IndexAny(normalized, ":@#"); subIdx != -1 {
			baseDep = normalized[:subIdx]
			versionPart = normalized[subIdx:]
		}
	}

	if baseDep != "" && versionPart != "" {
		if override, exists := overrides[baseDep]; exists {
			if overrideHasExplicitVersion(override) {
				return override
			}
			return override + versionPart
		}
	}

	meta, err := dependencyOverrideMeta(depStr)
	if err != nil || meta.IsLocalScheme() {
		return depStr
	}
	keys := make([]string, 0, len(overrides))
	for original := range overrides {
		keys = append(keys, original)
	}
	slices.Sort(keys)
	for _, original := range keys {
		source, err := dependencyOverrideMeta(original)
		if err != nil || source.IsLocalScheme() || !sameDependencyOverrideRepository(meta, source) {
			continue
		}
		if source.Path != "" && source.Path != meta.Path || source.Scheme != "" && source.Scheme != meta.Scheme {
			continue
		}
		if source.Tag != "" && source.Tag != meta.Tag ||
			source.Branch != "" && source.Branch != meta.Branch ||
			source.Commit != "" && source.Commit != meta.Commit {
			continue
		}
		replacement, err := dependencyOverrideMeta(overrides[original])
		if err != nil || replacement.IsLocalScheme() {
			return overrides[original]
		}
		if replacement.Tag == "" && replacement.Branch == "" && replacement.Commit == "" {
			replacement.Tag, replacement.Branch, replacement.Commit = meta.Tag, meta.Branch, meta.Commit
		}
		if replacement.Path == "" {
			replacement.Path = meta.Path
		}
		return formatDependencyOverride(depStr, meta, replacement)
	}

	return depStr
}

func dependencyOverrideMeta(reference string) (DependencyMeta, error) {
	if host, _, ok := strings.Cut(reference, "/"); ok && strings.EqualFold(host, "github.com") {
		reference = "https://" + reference
	}
	return explodeDependencyString(DependencyString(reference))
}

func sameDependencyOverrideRepository(a, b DependencyMeta) bool {
	if !strings.EqualFold(a.Site, b.Site) {
		return false
	}
	if strings.EqualFold(a.Site, "github.com") {
		return strings.EqualFold(a.User, b.User) && strings.EqualFold(a.Repo, b.Repo)
	}
	return a.User == b.User && a.Repo == b.Repo
}

func formatDependencyOverride(reference string, source, replacement DependencyMeta) string {
	path := replacement.User + "/" + replacement.Repo
	if replacement.Path != "" {
		path += "/" + replacement.Path
	}
	if replacement.Tag != "" {
		path += ":" + replacement.Tag
	} else if replacement.Branch != "" {
		path += "@" + replacement.Branch
	} else if replacement.Commit != "" {
		path += "#" + replacement.Commit
	}
	if replacement.Scheme != "" {
		return replacement.Scheme + "://" + path
	}
	if source.Scheme != "" {
		return source.Scheme + "://" + path
	}
	if replacement.SSH != "" {
		return replacement.SSH + "@" + replacement.Site + ":" + path
	}
	if source.SSH != "" {
		return source.SSH + "@" + replacement.Site + ":" + path
	}
	if strings.HasPrefix(reference, "https://") {
		return "https://" + replacement.Site + "/" + path
	}
	if strings.HasPrefix(reference, "http://") {
		return "http://" + replacement.Site + "/" + path
	}
	if !strings.EqualFold(replacement.Site, "github.com") {
		return "https://" + replacement.Site + "/" + path
	}
	return path
}

// ApplyDependencyOverridesWithLogging applies dependency overrides and logs when an override occurs
// Returns the overridden dependency string and whether an override was applied
func ApplyDependencyOverridesWithLogging(depStr string) (string, bool) {
	overriddenStr := ApplyDependencyOverrides(depStr)

	if overriddenStr != depStr {
		// An override was applied, log it
		print.Verb(fmt.Sprintf("dependency override: '%s' -> '%s'", depStr, overriddenStr))
		return overriddenStr, true
	}

	return depStr, false
}

// ResetDependencyOverrides resets the loaded overrides cache (mainly for testing)
func ResetDependencyOverrides() {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	loadedOverrides = nil
}

//nolint:lll
var (
	// MatchGitSSH matches ssh URLs such as 'git@github.com:Southclaws/sampctl'
	MatchGitSSH = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]+)\@((?:[a-zA-Z][a-zA-Z0-9\-]*\.)*[a-zA-Z][a-zA-Z0-9\-]*)\:((?:[A-Za-z0-9_\-\.]+\/?)*(?:[:@#].+)?)$`)
	// MatchDependencyString matches a dependency string such as 'Username/Repository:tag', 'Username/Repository@branch', 'Username/Repository#commit'
	MatchDependencyString = regexp.MustCompile(`^\/?([a-zA-Z0-9-]+)\/([a-zA-Z0-9-._]+)(?:\/)?([a-zA-Z0-9-_$\[\]{}().,\/]*)?((?:@)|(?:\:)|(?:#))?(.+)?$`)
	// MatchURLScheme matches URL-like dependency strings such as 'plugin://plugins/name', 'includes://legacy', 'filterscript://user/repo:tag'
	MatchURLScheme = regexp.MustCompile(`^(plugin|includes|filterscript|component):\/\/(.+)$`)
)

// Explode splits a dependency string into its component parts and returns a meta object
// a valid pattern is either a git URL or just a user/repo combination followed by an optional
// versioning string which is either a semantic version number or a SHA1 hash.
//
// Examples of valid dependency strings ignoring versioning:
//
//	https://github.com/user/repo
//	http://github.com/user/repo
//	github.com/user/repo
//	user/repo
//
// And, examples of valid dependency strings with the user/repo:version example followed by a
// description of what the constraint means.
// (More info: https://github.com/Masterminds/semver#basic-comparisons)
//
//	user/repo:1.2.3 (force version 1.2.3)
//	user/repo:1.2.x (allow any 1.2 build)
//	user/repo:2.x (allow any version 2 minor)
//
// And finally, examples of dependency strings with additional paths for when include files exist in
// a subdirectory of the repository.
//
//	https://github.com/user/repo/includes:1.2.3
//	http://github.com/user/repo/includes:1.2.3
//	github.com/user/repo/includes:1.2.3
//	user/repo/includes:1.2.3
//
// New URL-like scheme examples:
//
//	plugin://plugins/name (local plugin)
//	component://components/name (local component)
//	includes://legacy (local include directory)
//	filterscript://user/repo:tag (remote filterscript)
func (d DependencyString) Explode() (dep DependencyMeta, err error) {
	overriddenStr, _ := ApplyDependencyOverridesWithLogging(string(d))
	return dependencyOverrideMeta(overriddenStr)
}

// ExplodeWithOverrides parses a dependency using supplied overrides without loading configuration.
func (d DependencyString) ExplodeWithOverrides(overrides map[string]string) (DependencyMeta, error) {
	meta, err := dependencyOverrideMeta(applyDependencyOverrides(string(d), overrides))
	if err == nil && meta.IsURLScheme() {
		err = meta.Validate()
	}
	if err != nil {
		return DependencyMeta{}, err
	}
	return meta, nil
}

func explodeDependencyString(depStr DependencyString) (dep DependencyMeta, err error) {
	// First, check for URL-like schemes
	if MatchURLScheme.MatchString(string(depStr)) {
		return explodeURLScheme(string(depStr))
	}

	u, err := url.Parse(string(depStr))
	if err == nil {

		path := u.Path

		if u.Fragment != "" {
			path += "#" + u.Fragment
		}

		dep, err = explodePath(path)
		dep.Site = u.Host
	} else {
		user, host, path, success := attemptGitSSH(string(depStr))
		if success {
			dep, err = explodePath(path)
			dep.Site = host
			dep.SSH = user
		} else {
			dep, err = explodePath(string(depStr))
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

	return dep, nil
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

func explodeURLScheme(d string) (dep DependencyMeta, err error) {
	if !MatchURLScheme.MatchString(d) {
		err = errors.New("invalid dependency URL; expected scheme://owner/repository or scheme://local/path")
		return
	}

	captures := MatchURLScheme.FindStringSubmatch(d)
	if len(captures) != 3 {
		err = errors.New("could not parse dependency URL")
		return
	}

	dep.Scheme = captures[1]
	pathPart := captures[2]

	// Check if it's a local path by looking for "local/" prefix
	if strings.HasPrefix(pathPart, "local/") {
		// Local schemes like plugin://local/plugins/name or includes://local/legacy
		dep.Local = strings.TrimPrefix(pathPart, "local/")
	} else {
		// Remote schemes like filterscript://user/repo:tag
		dep.Site = "github.com"
		// Parse as if it's a user/repo with optional version
		remoteDep, err := explodePath(pathPart)
		if err != nil {
			return dep, err
		}
		dep.User = remoteDep.User
		dep.Repo = remoteDep.Repo
		dep.Path = remoteDep.Path
		dep.Tag = remoteDep.Tag
		dep.Branch = remoteDep.Branch
		dep.Commit = remoteDep.Commit
	}

	return dep, nil
}

func explodePath(d string) (dep DependencyMeta, err error) {
	if !MatchDependencyString.MatchString(d) {
		err = errors.New("invalid dependency; expected owner/repository with an optional :tag, @branch, or #commit")
		return
	}

	captures := MatchDependencyString.FindStringSubmatch(d)
	if len(captures) != 6 {
		err = errors.New("could not parse dependency reference")
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
				err = errors.Errorf("dependency commit hash must be 40 characters; got %d", len(captures[5]))
			}
			dep.Commit = captures[5]
		default:
			err = errors.New("dependency version must use :tag, @branch, or #commit")
		}
	} else if len(captures[5]) > 0 {
		// If there's text in the version part but no valid version specifier, it's invalid
		err = errors.New("dependency version must use :tag, @branch, or #commit")
	}
	return dep, err
}

// URL generates a GitHub URL for a package - it does not test the validity of the URL
func (dm DependencyMeta) URL() string {
	if dm.SSH != "" {
		return fmt.Sprintf("%s@%s:%s/%s", dm.SSH, dm.Site, dm.User, dm.Repo)
	}

	return fmt.Sprintf("https://%s/%s/%s", dm.Site, dm.User, dm.Repo)
}

// IsURLScheme returns true if this dependency uses a URL-like scheme
func (dm DependencyMeta) IsURLScheme() bool {
	return dm.Scheme != ""
}

// IsLocalScheme returns true if this dependency represents a local resource
func (dm DependencyMeta) IsLocalScheme() bool {
	return dm.Scheme != "" && dm.Local != ""
}

// IsRemoteScheme returns true if this dependency represents a remote resource with a scheme
func (dm DependencyMeta) IsRemoteScheme() bool {
	return dm.Scheme != "" && dm.Local == "" && dm.User != "" && dm.Repo != ""
}

// IsPlugin returns true if this dependency represents a plugin
func (dm DependencyMeta) IsPlugin() bool {
	return dm.Scheme == "plugin"
}

// IsIncludes returns true if this dependency represents an includes directory
func (dm DependencyMeta) IsIncludes() bool {
	return dm.Scheme == "includes"
}

// IsFilterscript returns true if this dependency represents a filterscript
func (dm DependencyMeta) IsFilterscript() bool {
	return dm.Scheme == "filterscript"
}
