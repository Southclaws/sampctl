package lockfile

import (
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// Resolver handles lockfile-aware dependency resolution
type Resolver struct {
	lockfile       *Lockfile
	dir            string
	format         string
	sampctlVersion string
	useLockfile    bool
	modified       bool
}

// NewResolver creates a new lockfile resolver
func NewResolver(dir, format, sampctlVersion string, useLockfile bool) (*Resolver, error) {
	resolver := &Resolver{
		dir:            dir,
		format:         format,
		sampctlVersion: sampctlVersion,
		useLockfile:    useLockfile,
	}

	if useLockfile {
		lf, err := Load(dir)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load lockfile")
		}

		if lf != nil {
			resolver.lockfile = lf
			print.Info("using lockfile with", lf.DependencyCount(), "locked dependencies")
		} else {
			resolver.lockfile = New(sampctlVersion)
			resolver.modified = true
			print.Verb("no lockfile found, will create new one")
		}
	}

	return resolver, nil
}

// GetLockedVersion returns the locked version for a dependency if available.
// If the lockfile doesn't have this dependency or useLockfile is false,
// it returns the original meta unchanged.
func (r *Resolver) GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta {
	if !r.useLockfile || r.lockfile == nil {
		return meta
	}

	key := DependencyKey(meta)
	locked, ok := r.lockfile.Dependencies[key]
	if !ok {
		return meta
	}

	// If we have a locked commit, use it for exact reproducibility
	if locked.Commit != "" {
		print.Verb(meta, "using locked commit", locked.Commit[:8])
		lockedMeta := meta
		lockedMeta.Commit = locked.Commit
		// Clear tag/branch since we're using exact commit
		lockedMeta.Tag = ""
		lockedMeta.Branch = ""
		return lockedMeta
	}

	return meta
}

// RecordResolution records the resolved version of a dependency
func (r *Resolver) RecordResolution(meta versioning.DependencyMeta, repo *git.Repository, transitive bool, requiredBy string) error {
	if !r.useLockfile || r.lockfile == nil {
		return nil
	}

	// Get the current HEAD commit
	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "failed to get repository HEAD")
	}

	commitSHA := head.Hash().String()
	key := DependencyKey(meta)

	// Check if this is already locked with the same commit
	existing, exists := r.lockfile.Dependencies[key]
	if exists && existing.Commit == commitSHA {
		// Already locked with same commit, just update required_by if transitive
		if transitive && requiredBy != "" {
			// Check if requiredBy is already in the list
			found := false
			for _, rb := range existing.RequiredBy {
				if rb == requiredBy {
					found = true
					break
				}
			}
			if !found {
				existing.RequiredBy = append(existing.RequiredBy, requiredBy)
				r.lockfile.Dependencies[key] = existing
				r.modified = true
			}
		}
		return nil
	}

	// Create the locked dependency
	locked := LockedDependency{
		Constraint: getConstraint(meta),
		Resolved:   getResolvedVersion(meta, repo),
		Commit:     commitSHA,
		Site:       meta.Site,
		User:       meta.User,
		Repo:       meta.Repo,
		Path:       meta.Path,
		Branch:     meta.Branch,
		Scheme:     meta.Scheme,
		Local:      meta.Local,
		Transitive: transitive,
	}

	if transitive && requiredBy != "" {
		locked.RequiredBy = []string{requiredBy}
	}

	// Calculate integrity hash for the dependency
	if meta.Repo != "" {
		// For now, use commit-based integrity
		locked.Integrity = CalculateCommitIntegrity(commitSHA)
	}

	r.lockfile.AddDependency(key, locked)
	r.modified = true

	print.Verb("locked", key, "at commit", commitSHA[:8])
	return nil
}

// RecordLocalDependency records a local (non-git) dependency
func (r *Resolver) RecordLocalDependency(meta versioning.DependencyMeta) error {
	if !r.useLockfile || r.lockfile == nil {
		return nil
	}

	if !meta.IsLocalScheme() {
		return nil
	}

	key := DependencyKey(meta)

	locked := LockedDependency{
		Scheme: meta.Scheme,
		Local:  meta.Local,
		User:   "local",
		Repo:   filepath.Base(meta.Local),
	}

	r.lockfile.AddDependency(key, locked)
	r.modified = true

	print.Verb("recorded local dependency", key)
	return nil
}

// Save persists the lockfile if it was modified
func (r *Resolver) Save() error {
	if !r.useLockfile || r.lockfile == nil {
		return nil
	}

	if !r.modified {
		print.Verb("lockfile unchanged, skipping save")
		return nil
	}

	return Save(r.dir, r.lockfile, r.format)
}

// ForceUpdate clears the lockfile to force fresh resolution
func (r *Resolver) ForceUpdate() {
	if r.lockfile != nil {
		r.lockfile = New(r.sampctlVersion)
		r.modified = true
		print.Info("lockfile cleared for fresh resolution")
	}
}

// HasLockfile returns true if a lockfile is loaded
func (r *Resolver) HasLockfile() bool {
	return r.lockfile != nil && r.lockfile.DependencyCount() > 0
}

// GetLockfile returns the current lockfile
func (r *Resolver) GetLockfile() *Lockfile {
	return r.lockfile
}

// IsLocked checks if a dependency is locked
func (r *Resolver) IsLocked(meta versioning.DependencyMeta) bool {
	if !r.useLockfile || r.lockfile == nil {
		return false
	}
	key := DependencyKey(meta)
	return r.lockfile.HasDependency(key)
}

// getResolvedVersion extracts the resolved version string
func getResolvedVersion(meta versioning.DependencyMeta, repo *git.Repository) string {
	// Try to get the current tag
	tag, err := versioning.GetRepoCurrentVersionedTag(repo)
	if err == nil && tag != nil {
		return tag.Name
	}

	// Fall back to constraint or HEAD
	switch {
	case meta.Tag != "":
		return meta.Tag
	case meta.Branch != "":
		return meta.Branch
	case meta.Commit != "":
		return meta.Commit[:8]
	default:
		return "HEAD"
	}
}

// PruneMissing removes dependencies from lockfile that are no longer in the package
func (r *Resolver) PruneMissing(currentDeps []versioning.DependencyMeta) {
	if !r.useLockfile || r.lockfile == nil {
		return
	}

	// Build set of current dependency keys
	currentKeys := make(map[string]bool)
	for _, dep := range currentDeps {
		currentKeys[DependencyKey(dep)] = true
	}

	// Find and remove stale entries
	for key := range r.lockfile.Dependencies {
		if !currentKeys[key] {
			print.Verb("pruning removed dependency from lockfile:", key)
			r.lockfile.RemoveDependency(key)
			r.modified = true
		}
	}
}