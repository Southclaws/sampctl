package lockfile

import (
	"path/filepath"
	"reflect"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type Resolver struct {
	lockfile       *Lockfile
	previous       *Lockfile
	dir            string
	sampctlVersion string
	useLockfile    bool
	modified       bool
}

func NewResolver(dir, sampctlVersion string, useLockfile bool) (*Resolver, error) {
	resolver := &Resolver{
		dir:            dir,
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
			print.Verb("using lockfile with", lf.DependencyCount(), "locked dependencies")
		} else {
			resolver.lockfile = New(sampctlVersion)
			resolver.modified = true
			print.Verb("no lockfile found, will create new one")
		}
	}

	return resolver, nil
}

func (r *Resolver) GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta {
	if !r.useLockfile || r.lockfile == nil {
		return meta
	}

	if r.lockfile.IsOutdated(meta) {
		print.Verb(meta, "lockfile constraint changed, resolving fresh version")
		return meta
	}

	key := DependencyKey(meta)
	locked, ok := r.lockfile.Dependencies[key]
	if !ok {
		return meta
	}

	if locked.Commit != "" {
		print.Verb(meta, "using locked commit", locked.Commit[:min(8, len(locked.Commit))])
		lockedMeta := meta
		lockedMeta.Commit = locked.Commit
		lockedMeta.Tag = ""
		lockedMeta.Branch = ""
		return lockedMeta
	}

	return meta
}

func (r *Resolver) GetPreviousDependency(meta versioning.DependencyMeta) (LockedDependency, bool) {
	if !r.useLockfile {
		return LockedDependency{}, false
	}

	if r.previous != nil {
		return r.previous.GetDependency(DependencyKey(meta))
	}

	if r.lockfile != nil {
		return r.lockfile.GetDependency(DependencyKey(meta))
	}

	return LockedDependency{}, false
}

func (r *Resolver) RecordResolution(meta versioning.DependencyMeta, resolution DependencyResolution, transitive bool, requiredBy string) error {
	if !r.useLockfile || r.lockfile == nil {
		return nil
	}

	commitSHA := resolution.Commit
	if commitSHA == "" {
		return errors.New("resolved dependency commit is empty")
	}

	resolvedVersion := resolution.Resolved
	if resolvedVersion == "" {
		resolvedVersion = defaultResolvedVersion(meta, commitSHA)
	}
	key := DependencyKey(meta)

	existing, exists := r.lockfile.Dependencies[key]
	if exists && existing.Commit == commitSHA {
		if existing.Constraint != getConstraint(meta) || existing.Transitive != transitive || existing.Resolved != resolvedVersion {
			existing.Constraint = getConstraint(meta)
			existing.Transitive = transitive
			existing.Resolved = resolvedVersion
			r.lockfile.Dependencies[key] = existing
			r.modified = true
		}
		if transitive && requiredBy != "" {
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

	locked := LockedDependency{
		Constraint: getConstraint(meta),
		Resolved:   resolvedVersion,
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

	if meta.Repo != "" {
		locked.Integrity = CalculateCommitIntegrity(commitSHA)
	}

	r.lockfile.AddDependency(key, locked)
	r.modified = true

	print.Verb("locked", key, "at commit", commitSHA[:8])
	return nil
}

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

func (r *Resolver) Save() error {
	if !r.useLockfile || r.lockfile == nil {
		return nil
	}

	existing, err := Load(r.dir)
	if err != nil {
		return err
	}

	if existing != nil && sameLockfileState(existing, r.lockfile) {
		r.modified = false
		r.previous = nil
		print.Verb("lockfile unchanged, skipping save")
		return nil
	}

	if err := Save(r.dir, r.lockfile); err != nil {
		return err
	}

	r.modified = false
	r.previous = nil

	return nil
}

func sameLockfileState(left, right *Lockfile) bool {
	l, r := *left, *right
	l.Generated = r.Generated
	return reflect.DeepEqual(l, r)
}

// Reload refreshes pins from disk while preserving an explicit force update.
func (r *Resolver) Reload() error {
	if !r.useLockfile {
		return nil
	}

	current, err := Load(r.dir)
	if err != nil {
		return err
	}

	if r.previous != nil {
		r.previous = current
		r.lockfile = New(r.sampctlVersion)
	} else if current != nil {
		r.lockfile = current
	} else {
		r.lockfile = New(r.sampctlVersion)
	}

	return nil
}

// Restore restores dependency pins after a failed project operation.
func (r *Resolver) Restore(current *Lockfile) {
	r.lockfile = current
	r.previous = nil
	r.modified = true
}

func (r *Resolver) ForceUpdate() {
	if r.lockfile != nil {
		r.previous = r.lockfile
		r.lockfile = New(r.sampctlVersion)
		r.modified = true
		print.Info("lockfile cleared for fresh resolution")
	}
}

func (r *Resolver) HasLockfile() bool {
	return r.lockfile != nil && r.lockfile.DependencyCount() > 0
}

func (r *Resolver) GetLockfile() *Lockfile {
	return r.lockfile
}

func (r *Resolver) IsLocked(meta versioning.DependencyMeta) bool {
	if !r.useLockfile || r.lockfile == nil {
		return false
	}
	key := DependencyKey(meta)
	return r.lockfile.HasDependency(key)
}

func (r *Resolver) PruneMissing(currentDeps []versioning.DependencyMeta) {
	if !r.useLockfile || r.lockfile == nil {
		return
	}

	currentKeys := make(map[string]bool)
	for _, dep := range currentDeps {
		currentKeys[DependencyKey(dep)] = true
	}

	for key := range r.lockfile.Dependencies {
		if !currentKeys[key] {
			print.Verb("pruning removed dependency from lockfile:", key)
			r.lockfile.RemoveDependency(key)
			r.modified = true
		}
	}
}

func (r *Resolver) RecordRuntime(version, platform, runtimeType string, files []LockedFileInfo) {
	if !r.useLockfile || r.lockfile == nil {
		return
	}
	r.lockfile.SetRuntime(version, platform, runtimeType, files)
	r.modified = true
	print.Verb("recorded runtime", version, "for", platform)
}

func (r *Resolver) RecordBuild(record BuildRecord) {
	if !r.useLockfile || r.lockfile == nil {
		return
	}
	r.lockfile.SetBuild(record)
	r.modified = true
	print.Verb("recorded build for", record.Entry)
}
