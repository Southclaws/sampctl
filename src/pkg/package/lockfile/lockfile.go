// Package lockfile provides lockfile support for reproducible dependency resolution.
// It stores resolved dependency versions with their exact commit SHAs and integrity hashes,
// ensuring consistent builds across different machines and time.
package lockfile

import (
	"fmt"
	"time"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// Version is the current lockfile format version
const Version = 1

// Filename constants for lockfile
const (
	// Filename is the standard lockfile name (format auto-detected from content)
	Filename = "pawn.lock"

	FilenameJSON = "pawn.lock"
	FilenameYAML = "pawn.lock"
)

// Lockfile represents the complete lockfile structure
type Lockfile struct {
	// Version is the lockfile format version
	Version int `json:"version" yaml:"version"`

	// Generated is the timestamp when this lockfile was created/updated
	Generated time.Time `json:"generated" yaml:"generated"`

	// SampctlVersion is the version of sampctl that generated this lockfile
	SampctlVersion string `json:"sampctl_version" yaml:"sampctl_version"`

	// Dependencies contains all resolved dependencies (direct and transitive)
	Dependencies map[string]LockedDependency `json:"dependencies" yaml:"dependencies"`
}

// LockedDependency represents a single locked dependency with its resolved version
type LockedDependency struct {
	// Original constraint from pawn.json (e.g., "1.2.x", "@master", "#abc123")
	Constraint string `json:"constraint" yaml:"constraint"`

	// Resolved is the actual version that was resolved (e.g., "1.2.5")
	Resolved string `json:"resolved" yaml:"resolved"`

	// Commit is the exact commit SHA that was checked out
	Commit string `json:"commit" yaml:"commit"`

	// Integrity is the SHA256 hash of the dependency content for verification
	Integrity string `json:"integrity,omitempty" yaml:"integrity,omitempty"`

	// Site is the git hosting site (default: github.com)
	Site string `json:"site,omitempty" yaml:"site,omitempty"`

	// User is the repository owner
	User string `json:"user" yaml:"user"`

	// Repo is the repository name
	Repo string `json:"repo" yaml:"repo"`

	// Path is an optional subdirectory within the repository
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// Branch is the branch name if dependency uses branch reference
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`

	// Transitive indicates if this is a transitive dependency
	Transitive bool `json:"transitive,omitempty" yaml:"transitive,omitempty"`

	// RequiredBy lists the packages that require this dependency (for transitive deps)
	RequiredBy []string `json:"required_by,omitempty" yaml:"required_by,omitempty"`

	// Scheme is the URL scheme if using special dependency types (plugin, includes, filterscript)
	Scheme string `json:"scheme,omitempty" yaml:"scheme,omitempty"`

	// Local is the local path for local scheme dependencies
	Local string `json:"local,omitempty" yaml:"local,omitempty"`
}

// New creates a new empty lockfile with the current version
func New(sampctlVersion string) *Lockfile {
	return &Lockfile{
		Version:        Version,
		Generated:      time.Now().UTC(),
		SampctlVersion: sampctlVersion,
		Dependencies:   make(map[string]LockedDependency),
	}
}

// DependencyKey generates a unique key for a dependency
func DependencyKey(meta versioning.DependencyMeta) string {
	if meta.Scheme != "" {
		if meta.Local != "" {
			return fmt.Sprintf("%s://local/%s", meta.Scheme, meta.Local)
		}
		return fmt.Sprintf("%s://%s/%s", meta.Scheme, meta.User, meta.Repo)
	}

	site := meta.Site
	if site == "" {
		site = "github.com"
	}
	return fmt.Sprintf("%s/%s/%s", site, meta.User, meta.Repo)
}

// AddDependency adds or updates a locked dependency
func (l *Lockfile) AddDependency(key string, dep LockedDependency) {
	if l.Dependencies == nil {
		l.Dependencies = make(map[string]LockedDependency)
	}
	l.Dependencies[key] = dep
}

// GetDependency retrieves a locked dependency by key
func (l *Lockfile) GetDependency(key string) (LockedDependency, bool) {
	dep, ok := l.Dependencies[key]
	return dep, ok
}

// HasDependency checks if a dependency exists in the lockfile
func (l *Lockfile) HasDependency(key string) bool {
	_, ok := l.Dependencies[key]
	return ok
}

// RemoveDependency removes a dependency from the lockfile
func (l *Lockfile) RemoveDependency(key string) {
	delete(l.Dependencies, key)
}

// GetLockedMeta returns a DependencyMeta with the locked commit SHA
// This is used to ensure we checkout the exact locked version
func (l *Lockfile) GetLockedMeta(meta versioning.DependencyMeta) (versioning.DependencyMeta, bool) {
	key := DependencyKey(meta)
	locked, ok := l.Dependencies[key]
	if !ok {
		return meta, false
	}

	// Create a new meta with the locked commit
	lockedMeta := meta
	if locked.Commit != "" {
		// Override with exact commit SHA for reproducibility
		lockedMeta.Commit = locked.Commit
		// Clear tag/branch since we're using exact commit
		lockedMeta.Tag = ""
		lockedMeta.Branch = ""
	}

	return lockedMeta, true
}

// IsOutdated checks if a dependency in the lockfile differs from the requested constraint
func (l *Lockfile) IsOutdated(meta versioning.DependencyMeta) bool {
	key := DependencyKey(meta)
	locked, ok := l.Dependencies[key]
	if !ok {
		return true // Not in lockfile means it needs to be added
	}

	// Get the constraint from the meta
	constraint := getConstraint(meta)

	// If constraint changed, it's outdated
	return locked.Constraint != constraint
}

// getConstraint extracts the constraint string from DependencyMeta
func getConstraint(meta versioning.DependencyMeta) string {
	switch {
	case meta.Tag != "":
		return ":" + meta.Tag
	case meta.Branch != "":
		return "@" + meta.Branch
	case meta.Commit != "":
		return "#" + meta.Commit
	default:
		return ""
	}
}

// UpdateTimestamp updates the generated timestamp to now
func (l *Lockfile) UpdateTimestamp() {
	l.Generated = time.Now().UTC()
}

// Validate checks if the lockfile is valid
func (l *Lockfile) Validate() error {
	if l.Version == 0 {
		return fmt.Errorf("lockfile version is not set")
	}
	if l.Version > Version {
		return fmt.Errorf("lockfile version %d is newer than supported version %d", l.Version, Version)
	}
	return nil
}

// DependencyCount returns the number of locked dependencies
func (l *Lockfile) DependencyCount() int {
	return len(l.Dependencies)
}

// DirectDependencies returns only direct (non-transitive) dependencies
func (l *Lockfile) DirectDependencies() map[string]LockedDependency {
	direct := make(map[string]LockedDependency)
	for key, dep := range l.Dependencies {
		if !dep.Transitive {
			direct[key] = dep
		}
	}
	return direct
}

// TransitiveDependencies returns only transitive dependencies
func (l *Lockfile) TransitiveDependencies() map[string]LockedDependency {
	transitive := make(map[string]LockedDependency)
	for key, dep := range l.Dependencies {
		if dep.Transitive {
			transitive[key] = dep
		}
	}
	return transitive
}