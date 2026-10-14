package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func checkManagedCache(ctx context.Context, opts checkOptions) ([]diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	access := inspectCheckCacheDirectory(opts.CacheDir)
	results := []diagnostic{access}
	if access.Status == checkFailed || access.Status == checkSkipped {
		return results, nil
	}

	for _, name := range []string{"compilers.json", "packages.json", "runtimes.json"} {
		if err := ctx.Err(); err != nil {
			return results, err
		}

		path := filepath.Join(opts.CacheDir, name)
		result, exists := inspectCheckCacheMetadata(opts.CacheDir, path)
		if !exists {
			continue
		}

		if opts.Fix && result.CanFix {
			var err error
			result, err = repairDiagnostic(ctx, result, func(ctx context.Context) error {
				return invalidateCheckCacheMetadata(ctx, opts.CacheDir, path)
			}, func() diagnostic {
				verified, _ := inspectCheckCacheMetadata(opts.CacheDir, path)
				return verified
			})
			if err != nil {
				return append(results, result), err
			}
		}

		results = append(results, result)
	}

	return results, nil
}

func inspectCheckCacheDirectory(path string) diagnostic {
	result := diagnostic{Name: "cache", Status: checkFailed, Path: path}
	result.Action = "Check access to the sampctl cache directory."

	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		result.Status, result.Message, result.Action = checkSkipped, "No sampctl cache yet", ""
		return result
	}
	if err != nil {
		result.Message, result.Err = "Cannot access the sampctl cache", err
		return result
	}

	info, err := os.Stat(path)
	if err != nil {
		result.Message, result.Err = "Cannot access the sampctl cache", err
		return result
	}

	if !info.IsDir() {
		result.Message = "The sampctl cache path is not a directory"
		return result
	}

	if _, err := os.ReadDir(path); err != nil {
		result.Message, result.Err = "Cannot read the sampctl cache", err
		return result
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&0o222 == 0 {
		result.Status = checkWarning
		result.Message = "The sampctl cache directory has no write permissions"
		result.Action = "Check the cache directory permissions before downloading files."
		return result
	}

	result.Status, result.Message, result.Action = checkPassed, "The sampctl cache is accessible", ""
	return result
}

func inspectCheckCacheMetadata(cacheDir, path string) (diagnostic, bool) {
	name := filepath.Base(path)
	result := diagnostic{Name: "cache metadata", Status: checkFailed, Path: path}

	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		result.Status, result.Message = checkPassed, name+" cache file is absent"
		return result, false
	}
	if err != nil {
		result.Message, result.Err = name+" cache cannot be accessed", err
		result.Action = "Check access to this cache file."
		return result, true
	}

	if !info.Mode().IsRegular() {
		result.Message = name + " cache is not a regular file"
		result.Action = "Check that this path points to a regular file."
		return result, true
	}

	invalid, err := validateCheckCacheMetadata(path)
	if err != nil {
		result.Message, result.Err = name+" cache cannot be read", err
		result.Action = "Check access to this cache file."
		if invalid {
			result.Message = name + " cache contains invalid metadata"
			if _, safeErr := checkManagedRegularPath(cacheDir, path); safeErr == nil {
				result.CanFix = true
				result.Action = "Run `sampctl check --fix` to remove this cache file. sampctl will download it again when needed."
			} else {
				result.Err = errors.Join(result.Err, safeErr)
				result.Action = "Check this path before removing the invalid cache file."
			}
		}
		return result, true
	}

	result.Status, result.Message = checkPassed, name+" cache is valid"
	return result, true
}

func validateCheckCacheMetadata(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	return validateCheckCacheMetadataData(filepath.Base(path), data)
}

func validateCheckCacheMetadataData(name string, data []byte) (bool, error) {
	var err error
	switch name {
	case "compilers.json":
		var metadata download.Compilers
		err = json.Unmarshal(data, &metadata)
		if err == nil {
			if len(metadata) == 0 {
				return true, fmt.Errorf("compiler metadata is empty")
			}
			platforms := make([]string, 0, len(metadata))
			for platform := range metadata {
				platforms = append(platforms, platform)
			}
			sort.Strings(platforms)
			for _, platform := range platforms {
				if err := validateCheckCompilerPackage(metadata[platform]); err != nil {
					return true, fmt.Errorf("compiler metadata for %s: %w", platform, err)
				}
			}
		}
	case "packages.json":
		var packages []pawnpackage.Package
		err = json.Unmarshal(data, &packages)
		if err == nil && packages == nil {
			return true, fmt.Errorf("package metadata must be an array")
		}
	case "runtimes.json":
		var metadata download.Runtimes
		err = json.Unmarshal(data, &metadata)
		if err == nil && metadata.Packages == nil {
			return true, fmt.Errorf("runtime metadata must contain a packages array")
		}
	default:
		return false, fmt.Errorf("unsupported metadata cache %q", name)
	}

	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	return errors.As(err, &syntaxError) || errors.As(err, &typeError), err
}

func invalidateCheckCacheMetadata(ctx context.Context, cacheDir, path string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	root, relative, original, err := openCheckManagedFileRoot(cacheDir, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()

	data, err := readCheckManagedFile(root, relative, original)
	if err != nil {
		return err
	}

	invalid, err := validateCheckCacheMetadataData(filepath.Base(path), data)
	if !invalid {
		if err != nil {
			return err
		}
		return fmt.Errorf("metadata cache changed and is now valid")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	current, err := root.Lstat(relative)
	if err != nil {
		return err
	}

	if !os.SameFile(original, current) || !current.Mode().IsRegular() {
		return fmt.Errorf("metadata cache changed during repair")
	}

	if err := root.Remove(relative); err != nil {
		return fmt.Errorf("invalidate %s cache: %w", filepath.Base(path), err)
	}

	return nil
}
