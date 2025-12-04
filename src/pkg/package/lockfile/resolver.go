package lockfile

import (
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type Resolver struct {
	lockfile       *Lockfile
	dir            string
	format         string
	sampctlVersion string
	useLockfile    bool
	modified       bool
}

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

func (r *Resolver) GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta {
	if !r.useLockfile || r.lockfile == nil {
		return meta
	}

	key := DependencyKey(meta)
	locked, ok := r.lockfile.Dependencies[key]
	if !ok {
		return meta
	}

	if locked.Commit != "" {
		print.Verb(meta, "using locked commit", locked.Commit[:8])
		lockedMeta := meta
		lockedMeta.Commit = locked.Commit
		lockedMeta.Tag = ""
		lockedMeta.Branch = ""
		return lockedMeta
	}

	return meta
}

func (r *Resolver) RecordResolution(meta versioning.DependencyMeta, repo *git.Repository, transitive bool, requiredBy string) error {
	if !r.useLockfile || r.lockfile == nil {
		return nil
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "failed to get repository HEAD")
	}

	commitSHA := head.Hash().String()
	key := DependencyKey(meta)

	existing, exists := r.lockfile.Dependencies[key]
	if exists && existing.Commit == commitSHA {
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

	if !r.modified {
		print.Verb("lockfile unchanged, skipping save")
		return nil
	}

	return Save(r.dir, r.lockfile, r.format)
}

func (r *Resolver) ForceUpdate() {
	if r.lockfile != nil {
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

func getResolvedVersion(meta versioning.DependencyMeta, repo *git.Repository) string {
	tag, err := versioning.GetRepoCurrentVersionedTag(repo)
	if err == nil && tag != nil {
		return tag.Name
	}

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

func (r *Resolver) RecordBuild(compilerVersion, compilerPreset, entry, output, outputHash string) {
	if !r.useLockfile || r.lockfile == nil {
		return
	}
	r.lockfile.SetBuild(compilerVersion, compilerPreset, entry, output, outputHash)
	r.modified = true
	print.Verb("recorded build for", entry)
}