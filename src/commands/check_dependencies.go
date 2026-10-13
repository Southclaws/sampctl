package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func checkDependencies(ctx context.Context, pkg pawnpackage.Package, lf *lockfile.Lockfile, overrides map[string]string, recoveryStages ...string) ([]diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pending := pkg.GetAllDependencies()
	visited := make(map[string]bool)
	results := checkDependencyStaging(pkg.Vendor, recoveryStages...)
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		declaration := pending[0]
		pending = pending[1:]
		meta, err := declaration.ExplodeWithOverrides(overrides)
		if err != nil {
			results = append(results, diagnostic{
				Name: "dependencies", Status: checkFailed,
				Message: fmt.Sprintf("Dependency %q is invalid", declaration), Action: "Correct the dependency in pawn.json or pawn.yaml.", Err: err,
			})
			continue
		}
		identity := meta.String() + "\x00" + meta.Path
		if visited[identity] {
			continue
		}
		visited[identity] = true
		result, subpackage := checkInstalledDependency(ctx, pkg, meta, lf)
		if err := ctx.Err(); err != nil {
			return results, err
		}
		results = append(results, result)
		if subpackage != nil {
			pending = append(pending, subpackage.Dependencies...)
		}
	}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	if len(results) == 0 {
		results = append(results, diagnostic{Name: "dependencies", Status: checkPassed, Message: "No dependencies are declared"})
	}
	return results, nil
}

func checkDependencyStaging(vendor string, recoveryStages ...string) []diagnostic {
	info, err := os.Lstat(vendor)
	if errors.Is(err, os.ErrNotExist) || err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil
	}
	var entries []os.DirEntry
	if err == nil {
		entries, err = os.ReadDir(vendor)
	}
	if err != nil {
		return []diagnostic{{Name: "dependency staging", Status: checkWarning, Message: "Cannot inspect temporary dependency directories", Path: vendor, Err: err, Action: "Check read access to the dependencies directory."}}
	}
	var results []diagnostic
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".sampctl-stage-") {
			continue
		}
		path := filepath.Join(vendor, entry.Name())
		if slices.Contains(recoveryStages, path) {
			continue
		}
		results = append(results, diagnostic{
			Name: "dependency staging", Status: checkWarning,
			Message: "Temporary dependency directory remains: " + entry.Name(), Path: path,
			Action: "Wait for dependency commands to finish. If the directory remains, use --verbose to find it. Keep any backups before deleting it.",
		})
	}
	return results
}

func checkInstalledDependency(ctx context.Context, pkg pawnpackage.Package, meta versioning.DependencyMeta, lf *lockfile.Lockfile) (diagnostic, *pawnpackage.Package) {
	result := diagnostic{Name: "dependencies", Status: checkFailed, Action: "Review local dependency changes, then run `sampctl ensure` from the project root."}
	if meta.IsLocalScheme() {
		return checkLocalDependency(pkg, meta), nil
	}
	result.Path = filepath.Join(pkg.Vendor, meta.Repo)
	if result = checkRemoteDependencyPath(result, meta); result.Status == checkFailed {
		return result, nil
	}
	if result = checkDependencyRepository(ctx, result, meta); result.Status == checkFailed {
		return result, nil
	}
	if result = checkDependencyRevision(ctx, result, meta, lf); result.Status == checkFailed {
		return result, nil
	}
	subpackage, err := readCheckDependencyDefinition(result.Path)
	if err == nil {
		err = subpackage.Validate()
	}
	if err != nil {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Dependency %s has invalid package configuration", meta)
		result.Err = err
		return result, nil
	}
	includePaths := []string{meta.Path}
	if !dependencyHasIncludeResources(subpackage) {
		includePaths = append(includePaths, subpackage.IncludePath)
	}
	for _, includePath := range includePaths {
		if result = checkDependencyIncludePath(result, meta, includePath); result.Status == checkFailed {
			return result, nil
		}
	}
	result.Message = fmt.Sprintf("Dependency %s is installed", meta)
	result.Action = ""
	return result, &subpackage
}

func dependencyHasIncludeResources(pkg pawnpackage.Package) bool {
	for _, res := range pkg.Resources {
		if len(res.Includes) > 0 {
			return true
		}
	}
	return false
}

func checkRemoteDependencyPath(result diagnostic, meta versioning.DependencyMeta) diagnostic {
	if meta.Repo == "." || meta.Repo == ".." {
		result.Message = fmt.Sprintf("Dependency %s has an invalid installation path", meta)
		result.Action = "Correct the dependency repository name in pawn.json or pawn.yaml."
		return result
	}
	info, err := os.Stat(result.Path)
	if err != nil {
		result.Message = fmt.Sprintf("Dependency %s is missing or inaccessible", meta)
		result.Err = err
		return result
	}
	if !info.IsDir() {
		result.Message = fmt.Sprintf("Dependency %s is not installed in a directory", meta)
		return result
	}
	if _, err := os.ReadDir(result.Path); err != nil {
		result.Message = fmt.Sprintf("Dependency %s installation cannot be read", meta)
		result.Err = err
		return result
	}
	result.Status = checkPassed
	return result
}

func checkDependencyIncludePath(result diagnostic, meta versioning.DependencyMeta, includePath string) diagnostic {
	if includePath == "" {
		return result
	}
	path := filepath.Join(result.Path, includePath)
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		err = errors.New("include path is not a directory")
	}
	if err == nil {
		_, err = os.ReadDir(path)
	}
	if err != nil {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Dependency %s include path is missing or invalid", meta)
		result.Err = err
	}
	return result
}

func checkLocalDependency(pkg pawnpackage.Package, meta versioning.DependencyMeta) diagnostic {
	path := filepath.Join(pkg.LocalPath, meta.Local)
	result := diagnostic{Name: "dependencies", Status: checkPassed, Message: fmt.Sprintf("Local dependency %s is available", meta), Path: path}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() && !info.Mode().IsRegular() {
		err = errors.New("dependency path is not a regular file or directory")
	}
	if err == nil {
		var file *os.File
		file, err = os.Open(path)
		if err == nil {
			err = file.Close()
		}
	}
	if err != nil {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Local dependency %s is missing or inaccessible", meta)
		result.Action = "Restore the local dependency or correct its configured path."
		result.Err = err
		return result
	}
	if meta.IsIncludes() && !info.IsDir() {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Local include dependency %s is not a directory", meta)
		result.Action = "Set the dependency path to an include directory."
	}
	if info.IsDir() && (meta.IsPlugin() || meta.Scheme == "component") {
		result = checkLocalDependencyDefinition(result, meta)
	}
	return result
}

func checkLocalDependencyDefinition(result diagnostic, meta versioning.DependencyMeta) diagnostic {
	subpackage, err := readCheckDependencyDefinition(result.Path)
	if err == nil {
		err = subpackage.Validate()
	}
	if err != nil {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Local dependency %s has invalid package configuration", meta)
		result.Err = err
	} else {
		result = checkDependencyIncludePath(result, meta, subpackage.IncludePath)
	}
	if result.Status == checkFailed {
		result.Action = "Correct the local dependency path or its package configuration."
	}
	return result
}

func readCheckDependencyDefinition(path string) (pawnpackage.Package, error) {
	for _, name := range []string{"pawn.json", "pawn.yaml"} {
		definition := filepath.Join(path, name)
		_, err := os.Lstat(definition)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return pawnpackage.Package{}, fmt.Errorf("inspect %s: %w", definition, err)
		}
		info, err := os.Stat(definition)
		if err != nil {
			return pawnpackage.Package{}, fmt.Errorf("inspect %s: %w", definition, err)
		}
		if !info.Mode().IsRegular() {
			return pawnpackage.Package{}, fmt.Errorf("package definition %s is not a regular file", definition)
		}
	}
	return pawnpackage.PackageFromDir(path)
}

func checkDependencyRepository(ctx context.Context, result diagnostic, meta versioning.DependencyMeta) diagnostic {
	info, err := os.Lstat(result.Path)
	if err != nil {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Dependency %s cannot be inspected", meta)
		result.Err = err
		return result
	}
	gitInfo, gitErr := os.Stat(filepath.Join(result.Path, ".git"))
	if gitErr == nil && !gitInfo.IsDir() && !gitInfo.Mode().IsRegular() {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Dependency %s has an invalid repository path", meta)
		return result
	}
	if info.Mode()&os.ModeSymlink != 0 && os.IsNotExist(gitErr) {
		err = lockfile.ValidateWorktreeFilesContext(ctx, result.Path)
	} else {
		err = validateCheckDependencyRepository(ctx, result.Path)
	}
	if err != nil {
		result.Status = checkFailed
		result.Message = fmt.Sprintf("Dependency %s installation is incomplete or invalid", meta)
		result.Err = err
		return result
	}
	result.Status = checkPassed
	return result
}

func validateCheckDependencyRepository(ctx context.Context, path string) error {
	repo, err := lockfile.OpenRepositoryContext(ctx, path)
	if err != nil {
		return err
	}
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("read repository HEAD: %w", err)
	}
	if _, err := repo.CommitObject(head.Hash()); err != nil {
		return fmt.Errorf("read HEAD commit: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := wt.Status(); err != nil {
		return fmt.Errorf("read repository status: %w", err)
	}
	return ctx.Err()
}

func checkDependencyRevision(ctx context.Context, result diagnostic, meta versioning.DependencyMeta, lf *lockfile.Lockfile) diagnostic {
	lockedMeta := normalizeCheckDependency(meta)
	var locked lockfile.LockedDependency
	if lf != nil && !lf.IsOutdated(lockedMeta) {
		locked, _ = lf.GetDependency(lockfile.DependencyKey(lockedMeta))
	}
	expectedCommit := meta.Commit
	if locked.Commit != "" {
		expectedCommit = locked.Commit
	}
	if expectedCommit != "" {
		repo, err := lockfile.OpenRepositoryContext(ctx, result.Path)
		if err == nil {
			head, headErr := repo.Head()
			err = headErr
			if err == nil && head.Hash().String() != expectedCommit {
				err = fmt.Errorf("installed HEAD %s differs from expected commit %s", head.Hash(), expectedCommit)
			}
		}
		if err != nil {
			result.Status = checkFailed
			result.Message = fmt.Sprintf("Dependency %s does not match its required revision", meta)
			result.Err = err
			return result
		}
	}
	if locked.Integrity != "" {
		matches, err := lockfile.VerifyIntegrityContext(ctx, result.Path, locked.Integrity)
		if err != nil || !matches {
			result.Status = checkFailed
			result.Message = fmt.Sprintf("Dependency %s does not match its locked integrity", meta)
			result.Err = err
		}
	}
	return result
}

func normalizeCheckDependency(meta versioning.DependencyMeta) versioning.DependencyMeta {
	if meta.IsRemoteScheme() {
		meta.Scheme = ""
	}
	return meta
}
