package commands

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func checkLockfile(pkg pawnpackage.Package, overrides map[string]string) (*lockfile.Lockfile, []diagnostic) {
	path := filepath.Join(pkg.LocalPath, lockfile.Filename)
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, []diagnostic{{Name: "lockfile", Status: checkSkipped, Message: "Optional pawn.lock is absent", Path: path}}
	}
	if err != nil {
		return nil, []diagnostic{lockfileIssue(path, "pawn.lock cannot be accessed", err)}
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, []diagnostic{lockfileIssue(path, "pawn.lock cannot be accessed", err)}
	}
	if !info.Mode().IsRegular() {
		return nil, []diagnostic{lockfileIssue(path, "pawn.lock is not a regular file", nil)}
	}
	lf, err := lockfile.Load(pkg.LocalPath)
	if err != nil {
		return nil, []diagnostic{lockfileIssue(path, "pawn.lock is malformed or unsupported", err)}
	}
	if lf == nil {
		return nil, []diagnostic{lockfileIssue(path, "pawn.lock could not be loaded", nil)}
	}
	if lf.Version < 1 {
		return nil, []diagnostic{lockfileIssue(path, "pawn.lock has an invalid version", fmt.Errorf("unsupported lockfile version %d", lf.Version))}
	}
	results := checkLockedDependencyData(lf, path)
	invalidData := len(results) > 0
	results = append(results, checkLockfileDeclarations(pkg, lf, path, overrides)...)
	if invalidData {
		return nil, results
	}
	if len(results) == 0 {
		results = append(results, diagnostic{Name: "lockfile", Status: checkPassed, Message: "Lockfile is valid", Path: path})
	}
	return lf, results
}

func lockfileIssue(path, message string, err error) diagnostic {
	return diagnostic{
		Name: "lockfile", Status: checkFailed, Message: message, Path: path, Err: err,
		Action: "Review and correct pawn.lock; to refresh a readable lockfile, run `sampctl ensure --lock-only` from the project root.",
	}
}

func checkLockedDependencyData(lf *lockfile.Lockfile, path string) []diagnostic {
	keys := make([]string, 0, len(lf.Dependencies))
	for key := range lf.Dependencies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var results []diagnostic
	for _, key := range keys {
		locked := lf.Dependencies[key]
		meta := versioning.DependencyMeta{Site: locked.Site, User: locked.User, Repo: locked.Repo, Path: locked.Path, Scheme: locked.Scheme, Local: locked.Local}
		if err := meta.Validate(); err != nil {
			results = append(results, lockfileIssue(path, fmt.Sprintf("Locked dependency %s has invalid metadata", key), err))
			continue
		}
		if key != lockfile.DependencyKey(meta) {
			results = append(results, lockfileIssue(path, fmt.Sprintf("Locked dependency %s does not match its metadata", key), nil))
			continue
		}
		if !meta.IsLocalScheme() && !validCheckHex(locked.Commit, 40) {
			results = append(results, lockfileIssue(path, fmt.Sprintf("Locked dependency %s has a missing or invalid revision", key), nil))
			continue
		}
		if err := checkLockedIntegrity(locked); err != nil {
			results = append(results, lockfileIssue(path, fmt.Sprintf("Locked dependency %s has invalid integrity data", key), err))
		}
	}
	return results
}

func checkLockedIntegrity(locked lockfile.LockedDependency) error {
	if !lockfile.IsValidIntegrity(locked.Integrity) {
		return fmt.Errorf("unsupported or malformed integrity %q", locked.Integrity)
	}
	if locked.Integrity == "" {
		return nil
	}
	kind, value := lockfile.ParseIntegrity(locked.Integrity)
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("integrity is not hexadecimal: %w", err)
	}
	if kind == "commit" && value != locked.Commit {
		return fmt.Errorf("integrity commit differs from locked revision")
	}
	return nil
}

func validCheckHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func checkLockfileDeclarations(pkg pawnpackage.Package, lf *lockfile.Lockfile, path string, overrides map[string]string) []diagnostic {
	if overrides == nil {
		return []diagnostic{{Name: "lockfile", Status: checkSkipped, Path: path, Message: "Lockfile dependency comparison requires valid dependency overrides"}}
	}
	declared := make(map[string]bool)
	var results []diagnostic
	validDeclarations := true
	for _, declaration := range pkg.GetAllDependencies() {
		meta, err := declaration.ExplodeWithOverrides(overrides)
		if err != nil {
			validDeclarations = false
			results = append(results, diagnostic{
				Name: "lockfile", Status: checkSkipped, Path: path, Err: err,
				Message: fmt.Sprintf("Lockfile comparison skipped for invalid dependency %q", declaration),
			})
			continue
		}
		meta = normalizeCheckDependency(meta)
		key := lockfile.DependencyKey(meta)
		declared[key] = true
		locked, present := lf.GetDependency(key)
		switch {
		case !present:
			results = append(results, lockfileIssue(path, fmt.Sprintf("Dependency %s is missing from pawn.lock", meta), nil))
		case lf.IsOutdated(meta):
			results = append(results, lockfileIssue(path, fmt.Sprintf("Dependency %s constraint differs from pawn.lock", meta), nil))
		case locked.Path != meta.Path:
			results = append(results, lockfileIssue(path, fmt.Sprintf("Dependency %s include path differs from pawn.lock", meta), nil))
		}
	}
	if validDeclarations {
		results = append(results, checkRemovedLockedDependencies(lf, declared, path)...)
	}
	return results
}

func checkRemovedLockedDependencies(lf *lockfile.Lockfile, declared map[string]bool, path string) []diagnostic {
	var removed []string
	for key, dependency := range lf.Dependencies {
		if !dependency.Transitive && !declared[key] {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	var results []diagnostic
	for _, key := range removed {
		results = append(results, lockfileIssue(path, fmt.Sprintf("Locked direct dependency %s is no longer declared", key), nil))
	}
	return results
}
