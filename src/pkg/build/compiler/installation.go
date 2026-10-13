package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	infrafs "github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

const compilerCompletionFilename = ".sampctl-compiler.json"

// InstallationError reports an invalid or interrupted managed compiler installation.
type InstallationError struct {
	Path            string
	Restored        bool
	Committed       bool
	RecoveryPending bool
	Err             error
}

func (e *InstallationError) Error() string {
	return fmt.Sprintf("compiler installation %s: %v", e.Path, e.Err)
}
func (e *InstallationError) Unwrap() error { return e.Err }

type compilerFile struct {
	Path string `json:"path"`
	Hash string `json:"sha256"`
	Mode uint32 `json:"mode"`
}

type compilerCompletion struct {
	Version  int                       `json:"format"`
	Meta     versioning.DependencyMeta `json:"compiler"`
	Platform string                    `json:"platform"`
	Package  download.Compiler         `json:"package"`
	Files    []compilerFile            `json:"files"`
}

// ManagedCompilerStatus describes a managed installation without changing files.
type ManagedCompilerStatus struct {
	Path            string
	Pending         bool
	Active          bool
	Committed       bool
	Valid           bool
	Legacy          bool
	CanRepair       bool
	HasCompletion   bool
	PermissionsOnly bool
}

// InspectManagedCompiler checks existing files and recovery metadata without downloading.
func InspectManagedCompiler(ctx context.Context, request CompilerFetchRequest) (status ManagedCompilerStatus, err error) {
	status.Path = request.Dir
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if err := compilerSafePath(request.Dir, false); err != nil {
		return status, &InstallationError{Path: request.Dir, Err: err}
	}
	lockPath := compilerLockPath(request.Dir)
	if info, statErr := os.Lstat(lockPath); statErr == nil {
		if !info.Mode().IsRegular() || !compilerSingleLink(info) {
			return status, &InstallationError{Path: request.Dir, Err: errors.New("compiler lock is not a private regular file")}
		}
		lock := flock.New(lockPath, flock.SetFlag(os.O_RDONLY))
		locked, lockErr := lock.TryRLock()
		if lockErr != nil {
			return status, errors.Join(lockErr, lock.Close())
		}
		if !locked {
			status.Active = true
			return status, lock.Close()
		}
		defer func() { err = errors.Join(err, lock.Close()) }()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return status, statErr
	}
	journal, err := readCompilerJournal(request.Dir)
	if err != nil {
		return status, &InstallationError{Path: request.Dir, Err: err}
	}
	if journal != nil {
		status.Pending, status.Committed = true, journal.Committed
		if err := validateCompilerRecovery(ctx, request.Dir, journal); err != nil {
			return status, &InstallationError{Path: request.Dir, Err: err}
		}
		status.CanRepair = true
		return status, nil
	}
	_, markerErr := os.Lstat(filepath.Join(request.Dir, compilerCompletionFilename))
	status.HasCompletion = !errors.Is(markerErr, os.ErrNotExist)
	pkg, err := readCompilerPackage(request.CacheDir, request.Platform)
	if err != nil {
		return status, err
	}
	err = verifyCompilerInstallation(ctx, request.Dir, request.Meta, request.Platform, pkg)
	if err == nil {
		status.Valid = true
		return status, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		if _, markerErr := os.Lstat(filepath.Join(request.Dir, compilerCompletionFilename)); errors.Is(markerErr, os.ErrNotExist) && validateCompilerFiles(request.Dir, pkg) == nil {
			status.Valid, status.Legacy = true, true
			return status, nil
		}
	}
	if completion, completionErr := readCompilerCompletion(request.Dir); completionErr == nil && reflect.DeepEqual(completion.Meta, request.Meta) && completion.Platform == request.Platform && reflect.DeepEqual(completion.Package, pkg) {
		_, permissionsOnly, permissionErr := compilerPermissionRepairMode(ctx, request.Dir, completion)
		status.PermissionsOnly = permissionErr == nil && permissionsOnly
		err = errors.Join(err, permissionErr)
	}
	fetcher := &compilerPackageFetcher{meta: request.Meta, platform: request.Platform, cacheDir: request.CacheDir, compiler: pkg}
	if hit, assetPath := fetcher.resource(nil).Cached(request.Meta.Tag); hit {
		inventory, archiveErr := compilerArchiveInventory(ctx, assetPath, pkg)
		if archiveErr == nil && compilerReplacementAllowed(ctx, request.Dir, inventory) == nil {
			status.CanRepair = true
		}
	}
	return status, &InstallationError{Path: request.Dir, Err: err}
}

// RepairManagedCompiler recovers an interruption or reinstalls from a verified cached archive.
// It never downloads a compiler or changes the selected version.
func RepairManagedCompiler(ctx context.Context, request CompilerFetchRequest) (err error) {
	unlock, err := lockCompilerInstallation(ctx, request.Dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	if err := recoverCompilerInstallation(ctx, request.Dir); err != nil {
		return err
	}
	pkg, err := readCompilerPackage(request.CacheDir, request.Platform)
	if err != nil {
		return err
	}
	if err := verifyCompilerInstallation(ctx, request.Dir, request.Meta, request.Platform, pkg); err == nil {
		return nil
	}
	fetcher := &compilerPackageFetcher{meta: request.Meta, platform: request.Platform, cacheDir: request.CacheDir, compiler: pkg}
	hit, assetPath := fetcher.resource(nil).Cached(request.Meta.Tag)
	if !hit {
		return &InstallationError{Path: request.Dir, Err: errors.New("compiler archive is not cached")}
	}
	return installCompilerArchive(ctx, assetPath, request.Dir, request.Meta, request.Platform, pkg)
}

// RepairManagedCompilerPermissions restores execute permission under the installation lock.
func RepairManagedCompilerPermissions(ctx context.Context, dir, binary string) (err error) {
	if !compilerRelativePath(binary) {
		return errors.New("invalid compiler executable path")
	}
	unlock, err := lockCompilerInstallation(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	path := filepath.Join(dir, binary)
	info, err := compilerRegularFile(path)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	file, err := root.OpenFile(binary, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	current, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || !compilerSingleLink(current) {
		return errors.New("compiler executable changed during permission repair")
	}
	mode := current.Mode().Perm() | 0o111
	completion, markerErr := readCompilerCompletion(dir)
	if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
		return markerErr
	}
	if completion != nil {
		if binary != completion.Package.Binary {
			return errors.New("compiler executable does not match its completion metadata")
		}
		if err := verifyCompilerInstallation(ctx, dir, completion.Meta, completion.Platform, completion.Package); err == nil {
			return nil
		}
		recordedMode, canRepair, inspectErr := compilerPermissionRepairMode(ctx, dir, completion)
		if inspectErr != nil {
			return inspectErr
		}
		if !canRepair {
			return errors.New("compiler files changed before permission repair")
		}
		mode = recordedMode
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	return nil
}

func compilerPermissionRepairMode(ctx context.Context, dir string, completion *compilerCompletion) (os.FileMode, bool, error) {
	if err := validateCompilerPackage(completion.Package); err != nil {
		return 0, false, err
	}
	inventory, err := compilerInventory(ctx, dir, false)
	if err != nil {
		return 0, false, err
	}
	var mode os.FileMode
	for i, entry := range inventory {
		if entry.Path != completion.Package.Binary || entry.Mode&0o111 != 0 {
			continue
		}
		for _, expected := range completion.Files {
			if expected.Path == entry.Path && expected.Hash == entry.Hash && expected.Mode&0o111 != 0 {
				mode = os.FileMode(expected.Mode)
				inventory[i].Mode = expected.Mode
			}
		}
	}
	if mode == 0 || !reflect.DeepEqual(inventory, completion.Files) {
		return 0, false, nil
	}
	if err := compilerReplacementAllowed(ctx, dir, completion.Files); err != nil {
		return 0, false, err
	}
	return mode, true, nil
}

func readCompilerPackage(cacheDir, platform string) (download.Compiler, error) {
	path := filepath.Join(cacheDir, "compilers.json")
	if _, err := compilerRegularFile(path); err != nil {
		return download.Compiler{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return download.Compiler{}, err
	}
	var packages download.Compilers
	if err := json.Unmarshal(data, &packages); err != nil {
		return download.Compiler{}, err
	}
	pkg, ok := packages[platform]
	if !ok {
		return pkg, fmt.Errorf("compiler metadata is unavailable for platform %s", platform)
	}
	return pkg, validateCompilerPackage(pkg)
}

func validateCompilerPackage(pkg download.Compiler) error {
	if download.ExtractFuncFromName(pkg.Method) == nil {
		return fmt.Errorf("invalid extract type: %s", pkg.Method)
	}
	if _, err := regexp.Compile(pkg.Match); err != nil {
		return fmt.Errorf("invalid compiler archive matcher: %w", err)
	}
	if !compilerRelativePath(pkg.Binary) {
		return errors.New("invalid compiler executable path")
	}
	for pattern, target := range pkg.Paths {
		if pattern == "" {
			return errors.New("compiler archive path matcher is empty")
		}
		if target != "" && !compilerRelativePath(strings.TrimSuffix(target, "/")) {
			return errors.New("invalid compiler supporting file path")
		}
	}
	return nil
}

func compilerRelativePath(path string) bool {
	return path != "" && path != "." && !strings.ContainsAny(path, "\\:") && !filepath.IsAbs(path) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) == path && path != ".." && !strings.HasPrefix(path, "../")
}

func compilerSafePath(path string, create bool) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var parents []string
	for current := path; ; current = filepath.Dir(current) {
		parents = append(parents, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	for i := len(parents) - 1; i >= 0; i-- {
		info, err := os.Lstat(parents[i])
		if errors.Is(err, os.ErrNotExist) {
			if create && i > 0 {
				if err := os.Mkdir(parents[i], 0o700); err != nil && !errors.Is(err, os.ErrExist) {
					return err
				}
				continue
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("compiler path %s must be a directory without symbolic links", parents[i])
		}
	}
	return nil
}

func compilerRegularFile(path string) (os.FileInfo, error) {
	if err := compilerSafePath(filepath.Dir(path), false); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("compiler file %s must be a regular file with one hard link", path)
	}
	singleLink, linkErr := compilerPathSingleLink(path, info)
	if linkErr != nil {
		return nil, linkErr
	}
	if !singleLink {
		return nil, fmt.Errorf("compiler file %s must have one hard link", path)
	}
	return info, nil
}

func compilerLockPath(dir string) string {
	return filepath.Join(filepath.Dir(dir), "."+filepath.Base(dir)+".sampctl-compiler.lock")
}

func lockCompilerInstallation(ctx context.Context, dir string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := compilerSafePath(dir, true); err != nil {
		return nil, &InstallationError{Path: dir, Err: err}
	}
	path := compilerLockPath(dir)
	if _, err := compilerRegularFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lock := flock.New(path)
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil || !locked {
		return nil, errors.Join(err, ctx.Err(), lock.Close())
	}
	return lock.Close, nil
}

func lockCompilerRead(ctx context.Context, dir string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := compilerSafePath(dir, false); err != nil {
		return nil, &InstallationError{Path: dir, Err: err}
	}
	path := compilerLockPath(dir)
	if _, err := compilerRegularFile(path); err != nil {
		return nil, err
	}
	lock := flock.New(path, flock.SetFlag(os.O_RDONLY))
	locked, err := lock.TryRLockContext(ctx, 25*time.Millisecond)
	if err != nil || !locked {
		return nil, errors.Join(err, ctx.Err(), lock.Close())
	}
	return lock.Close, nil
}

func installedCompilerFromCache(ctx context.Context, request CompilerFetchRequest) (pkg download.Compiler, hit bool, err error) {
	if err := ctx.Err(); err != nil {
		return pkg, false, err
	}
	if request.Meta.Tag == "" {
		return pkg, false, nil
	}
	if _, markerErr := os.Lstat(filepath.Join(request.Dir, compilerCompletionFilename)); errors.Is(markerErr, os.ErrNotExist) {
		return pkg, false, nil
	}
	if _, lockErr := os.Lstat(compilerLockPath(request.Dir)); errors.Is(lockErr, os.ErrNotExist) {
		return pkg, false, nil
	}
	unlock, err := lockCompilerRead(ctx, request.Dir)
	if err != nil {
		return pkg, false, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	journal, err := readCompilerJournal(request.Dir)
	if err != nil || journal != nil {
		return pkg, false, err
	}
	pkg, err = readCompilerPackage(request.CacheDir, request.Platform)
	if err != nil {
		return download.Compiler{}, false, ctx.Err()
	}
	if err := verifyCompilerInstallation(ctx, request.Dir, request.Meta, request.Platform, pkg); err != nil {
		return download.Compiler{}, false, ctx.Err()
	}
	return pkg, true, nil
}

func validateCompilerFiles(dir string, pkg download.Compiler) error {
	paths := []string{pkg.Binary}
	for _, target := range pkg.Paths {
		if target != "" && !strings.HasSuffix(target, "/") {
			paths = append(paths, target)
		}
	}
	for _, path := range paths {
		if !compilerRelativePath(path) {
			return errors.New("invalid compiler file path")
		}
		info, err := compilerRegularFile(filepath.Join(dir, path))
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			return fmt.Errorf("compiler file %s is empty", path)
		}
	}
	return nil
}

func readCompilerCompletion(dir string) (*compilerCompletion, error) {
	path := filepath.Join(dir, compilerCompletionFilename)
	if _, err := compilerRegularFile(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var completion compilerCompletion
	if err := json.Unmarshal(data, &completion); err != nil {
		return nil, err
	}
	if completion.Version != 1 || len(completion.Files) == 0 {
		return nil, errors.New("invalid compiler completion metadata")
	}
	seen := make(map[string]bool)
	for _, file := range completion.Files {
		if !compilerRelativePath(file.Path) || file.Path == compilerCompletionFilename || len(file.Hash) != sha256.Size*2 || seen[file.Path] {
			return nil, errors.New("invalid compiler file inventory")
		}
		if _, err := hex.DecodeString(file.Hash); err != nil {
			return nil, errors.New("invalid compiler file fingerprint")
		}
		seen[file.Path] = true
	}
	return &completion, nil
}

func verifyCompilerInstallation(ctx context.Context, dir string, meta versioning.DependencyMeta, platform string, pkg download.Compiler) error {
	completion, err := readCompilerCompletion(dir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(completion.Meta, meta) || completion.Platform != platform || !reflect.DeepEqual(completion.Package, pkg) {
		return errors.New("compiler completion metadata does not match the selected compiler")
	}
	files, err := compilerInventory(ctx, dir, false)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(files, completion.Files) {
		return errors.New("compiler files do not match the completed installation")
	}
	if err := validateCompilerFiles(dir, pkg); err != nil {
		return err
	}
	if runtime.GOOS != "windows" && infrafs.IsPosixPlatform(platform) {
		info, err := compilerRegularFile(filepath.Join(dir, pkg.Binary))
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o111 == 0 {
			return errors.New("compiler executable does not have execute permission")
		}
	}
	return nil
}

func compilerInventory(ctx context.Context, dir string, includeMarker bool) ([]compilerFile, error) {
	if err := compilerSafePath(dir, false); err != nil {
		return nil, err
	}
	var inventory []compilerFile
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if !includeMarker && relative == compilerCompletionFilename {
			return nil
		}
		info, err := compilerRegularFile(path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, &compilerContextReader{ctx: ctx, reader: file})
		err = errors.Join(err, file.Close())
		if err != nil {
			return err
		}
		inventory = append(inventory, compilerFile{Path: filepath.ToSlash(relative), Hash: hex.EncodeToString(hash.Sum(nil)), Mode: uint32(info.Mode().Perm())})
		return nil
	})
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].Path < inventory[j].Path })
	return inventory, err
}

func compilerReplacementAllowed(ctx context.Context, dir string, replacement []compilerFile) error {
	files, err := compilerInventory(ctx, dir, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	completion, markerErr := readCompilerCompletion(dir)
	if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
		return fmt.Errorf("cannot verify compiler ownership: %w", markerErr)
	}
	allowed := make(map[string]compilerFile)
	if completion != nil {
		for _, file := range completion.Files {
			allowed[file.Path] = file
		}
	} else {
		for _, file := range replacement {
			allowed[file.Path] = file
		}
	}
	for _, file := range files {
		expected, ok := allowed[file.Path]
		if !ok || (completion == nil && file.Hash != expected.Hash) {
			return fmt.Errorf("compiler cache contains an unverified file %s", file.Path)
		}
	}
	directories := make(map[string]bool)
	for path := range allowed {
		for parent := filepath.Dir(filepath.FromSlash(path)); parent != "."; parent = filepath.Dir(parent) {
			directories[filepath.ToSlash(parent)] = true
		}
	}
	return filepath.WalkDir(dir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == dir || !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, current)
		if err != nil {
			return err
		}
		if !directories[filepath.ToSlash(relative)] {
			return fmt.Errorf("compiler cache contains an unverified directory %s", relative)
		}
		return nil
	})
}
