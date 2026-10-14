package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

const (
	runtimeManifestFileName          = "sampctl-runtime-manifest.json"
	installedRuntimeManifestFileName = ".sampctl-runtime.json"
	runtimeStagingDir                = "runtime_staging"
)

var runtimeManifestRelativePath = runtimeManifestFileName

type runtimeManifest struct {
	Version     string            `json:"version"`
	Platform    string            `json:"platform"`
	RuntimeType run.RuntimeType   `json:"runtime_type"`
	Files       []runtimeFileInfo `json:"files"`
}

type runtimeFileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
	Mode uint32 `json:"mode"`
}

func (m runtimeManifest) matchesRuntime(cfg run.Runtime) bool {
	return m.Version == cfg.Version &&
		m.Platform == cfg.Platform &&
		m.RuntimeType == cfg.GetEffectiveRuntimeType()
}

func buildRuntimeManifest(root string, cfg run.Runtime) (runtimeManifest, error) {
	manifest := runtimeManifest{
		Version:     cfg.Version,
		Platform:    cfg.Platform,
		RuntimeType: cfg.GetEffectiveRuntimeType(),
	}

	manifestRel := filepath.ToSlash(runtimeManifestRelativePath)

	err := filepath.WalkDir(root, func(path string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			relPath = ""
		}
		if d.IsDir() {
			return nil
		}

		if strings.EqualFold(relPath, manifestRel) || isRuntimeStatePath(relPath) {
			return nil
		}

		if !d.Type().IsRegular() {
			return errors.Wrapf(os.ErrInvalid, "runtime file %s must be a regular file", relPath)
		}

		hash, size, err := hashFile(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, runtimeFileInfo{
			Path: relPath,
			Size: size,
			Hash: hash,
			Mode: uint32(info.Mode()),
		})

		return nil
	})
	if err != nil {
		return runtimeManifest{}, err
	}

	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	})

	return manifest, nil
}

func hashFile(path string) (hash string, size int64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	h := sha256.New()
	size, err = io.Copy(h, file)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func writeRuntimeManifest(path string, manifest runtimeManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return fs.WriteFileAtomic(path, data, 0o755, 0o600)
}

func readRuntimeManifest(path string) (runtimeManifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return runtimeManifest{}, err
	}

	var manifest runtimeManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return runtimeManifest{}, err
	}

	return manifest, nil
}

func verifyRuntimeManifest(manifest runtimeManifest, root string) error {
	if len(manifest.Files) == 0 {
		return errors.Wrap(os.ErrInvalid, "runtime manifest contains no files")
	}

	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		fullPath, err := runtimeFilePath(root, file.Path)
		if err != nil {
			return err
		}

		key := strings.ToLower(filepath.Clean(fullPath))
		if _, exists := seen[key]; exists {
			return errors.Wrapf(os.ErrInvalid, "duplicate runtime file %s", file.Path)
		}
		seen[key] = struct{}{}

		info, err := os.Lstat(fullPath)
		if err != nil {
			return errors.Wrapf(err, "missing runtime file %s", file.Path)
		}
		if !info.Mode().IsRegular() {
			return errors.Wrapf(os.ErrInvalid, "runtime file %s must be a regular file", file.Path)
		}
		if info.Size() != file.Size {
			return errors.Errorf("runtime file %s size mismatch", file.Path)
		}
		if fs.IsPosixPlatform(manifest.Platform) && os.FileMode(file.Mode).Perm() != info.Mode().Perm() {
			return errors.Errorf("runtime file %s permissions mismatch", file.Path)
		}

		hash, _, err := hashFile(fullPath)
		if err != nil {
			return errors.Wrapf(err, "failed to hash runtime file %s", file.Path)
		}
		if hash != file.Hash {
			return errors.Errorf("runtime file %s checksum mismatch", file.Path)
		}
	}

	return nil
}

func copyRuntimeFilesContext(ctx context.Context, manifest runtimeManifest, srcRoot, destRoot string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := verifyRuntimeManifest(manifest, srcRoot); err != nil {
		return err
	}

	// Validate every destination before replacing any existing runtime file.
	for _, file := range manifest.Files {
		if _, err := runtimeFilePath(destRoot, file.Path); err != nil {
			return err
		}
	}

	for _, file := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}

		src := filepath.Join(srcRoot, filepath.FromSlash(file.Path))
		dest, err := runtimeFilePath(destRoot, file.Path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return errors.Wrap(err, "failed to create runtime destination directory")
		}

		if err := copyFileWithModeContext(ctx, src, dest, os.FileMode(file.Mode)); err != nil {
			return errors.Wrapf(err, "failed to copy runtime file %s", file.Path)
		}
	}

	return nil
}

func copyFileWithModeContext(ctx context.Context, src, dest string, mode os.FileMode) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, in.Close()) }()

	return fs.WriteFromReaderAtomic(dest, runtimeCopyReader{ctx: ctx, reader: in}, 0o755, mode.Perm())
}

type runtimeCopyReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r runtimeCopyReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	n, err := r.reader.Read(p)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, stderrors.Join(err, ctxErr)
	}

	return n, err
}

func runtimeFilePath(root, relative string) (string, error) {
	if root == "" {
		return "", errors.Wrap(os.ErrInvalid, "runtime installation directory is empty")
	}

	local := filepath.FromSlash(relative)
	if !filepath.IsLocal(local) || local == "." {
		return "", errors.Wrapf(os.ErrInvalid, "runtime path %q must stay inside its installation", relative)
	}

	root = filepath.Clean(root)
	full := filepath.Join(root, local)
	for path := full; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || (path != full && !info.IsDir()) {
				return "", errors.Wrapf(os.ErrInvalid, "runtime path %s contains a symlink or non-directory", path)
			}
			if path == full && !info.Mode().IsRegular() {
				return "", errors.Wrapf(os.ErrInvalid, "runtime file %s must be a regular file", relative)
			}
		}
		if path == root {
			break
		}
	}

	return full, nil
}

func removeRuntimeFiles(manifest runtimeManifest, root string) error {
	for _, file := range manifest.Files {
		if _, err := runtimeFilePath(root, file.Path); err != nil {
			return err
		}
	}

	dirSet := map[string]struct{}{}
	for _, file := range manifest.Files {
		target, err := runtimeFilePath(root, file.Path)
		if err != nil {
			return err
		}
		if fs.Exists(target) {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return errors.Wrapf(err, "failed to remove runtime file %s", file.Path)
			}
		}
		dirSet[filepath.Dir(target)] = struct{}{}
	}

	dirs := make([]string, 0, len(dirSet))
	for dir := range dirSet {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	for _, dir := range dirs {
		if dir == root || dir == "." {
			continue
		}
		if err := removeEmptyRuntimeDir(dir); err != nil {
			return err
		}
	}

	return nil
}

func manifestsEqual(a, b runtimeManifest) bool {
	if a.Version != b.Version || a.Platform != b.Platform || a.RuntimeType != b.RuntimeType {
		return false
	}

	if len(a.Files) != len(b.Files) {
		return false
	}

	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			return false
		}
	}

	return true
}

func removeEmptyRuntimeDir(dir string) error {
	err := os.Remove(dir)
	if err == nil || os.IsNotExist(err) {
		return nil
	}

	entries, readErr := os.ReadDir(dir)
	if readErr == nil && len(entries) > 0 {
		return nil
	}

	if os.IsNotExist(readErr) {
		return nil
	}

	if readErr != nil {
		return errors.Wrapf(readErr, "failed to inspect runtime directory %s", dir)
	}

	return errors.Wrapf(err, "failed to remove runtime directory %s", dir)
}

func runtimeManifestPath(root string) string {
	return filepath.Join(root, runtimeManifestRelativePath)
}

func loadInstalledRuntimeManifest(root string) (*runtimeManifest, error) {
	// Runtime ownership survives project rollback and also applies when the
	// installation lives outside the project or pawn.lock is disabled.
	path, err := runtimeFilePath(root, installedRuntimeManifestFileName)
	if err != nil {
		return nil, err
	}

	manifest, err := readRuntimeManifest(path)
	if err == nil {
		if err := validateInstalledRuntimeManifest(manifest, root); err != nil {
			return nil, err
		}
		return &manifest, nil
	}

	if !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "failed to read installed runtime manifest")
	}

	if _, err := runtimeFilePath(root, lockfile.Filename); err != nil {
		return nil, err
	}

	lf, err := lockfile.Load(root)
	if err != nil {
		return nil, err
	}
	if lf != nil && lf.Runtime != nil {
		manifest := runtimeManifestFromLockedRuntime(lf.Runtime)
		return &manifest, nil
	}

	return nil, nil
}

func validateInstalledRuntimeManifest(manifest runtimeManifest, root string) error {
	if len(manifest.Files) == 0 || manifest.Version == "" || manifest.Platform == "" {
		return errors.Wrap(os.ErrInvalid, "installed runtime manifest is incomplete")
	}

	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		if file.Path != filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path))) {
			return errors.Wrapf(os.ErrInvalid, "runtime path %q is not normalized", file.Path)
		}

		path, err := runtimeFilePath(root, file.Path)
		if err != nil {
			return err
		}

		key := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[key]; exists || isRuntimeStatePath(file.Path) {
			return errors.Wrapf(os.ErrInvalid, "invalid installed runtime entry %s", file.Path)
		}
		seen[key] = struct{}{}

		hash, err := hex.DecodeString(file.Hash)
		if err != nil || len(hash) != sha256.Size || file.Size < 0 || file.Mode > 0o777 {
			return errors.Wrapf(os.ErrInvalid, "invalid installed runtime fingerprint for %s", file.Path)
		}
	}

	return nil
}

func isRuntimeStatePath(relative string) bool {
	first := strings.ToLower(strings.Split(filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative))), "/")[0])
	return first == runtimeManifestFileName || first == installedRuntimeManifestFileName || first == RuntimeRecoveryFilename || first == runtimeLockFilename || strings.HasPrefix(first, runtimeOperationPrefix)
}

func writeInstalledRuntimeManifest(ctx context.Context, root string, manifest runtimeManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := runtimeFilePath(root, installedRuntimeManifestFileName)
	if err != nil {
		return err
	}

	previous, err := readRuntimeManifest(path)
	if err == nil && manifestsEqual(previous, manifest) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return writeRuntimeManifest(path, manifest)
}

type RuntimeFileInfo struct {
	Path string
	Size int64
	Hash string
	Mode uint32
}

type RuntimeManifestInfo struct {
	Version     string
	Platform    string
	RuntimeType string
	Files       []RuntimeFileInfo
}

func GetRuntimeManifestInfo(workingDir string) (*RuntimeManifestInfo, error) {
	manifest, err := loadInstalledRuntimeManifest(workingDir)
	if err != nil {
		return nil, err
	}

	if manifest == nil {
		return nil, nil
	}

	return runtimeManifestToInfo(*manifest), nil
}

func runtimeManifestToInfo(manifest runtimeManifest) *RuntimeManifestInfo {
	info := &RuntimeManifestInfo{
		Version:     manifest.Version,
		Platform:    manifest.Platform,
		RuntimeType: string(manifest.RuntimeType),
		Files:       make([]RuntimeFileInfo, len(manifest.Files)),
	}

	for i, f := range manifest.Files {
		info.Files[i] = RuntimeFileInfo(f)
	}

	return info
}

func runtimeManifestFromLockedRuntime(runtimeInfo *lockfile.LockedRuntime) runtimeManifest {
	manifest := runtimeManifest{
		Version:     runtimeInfo.Version,
		Platform:    runtimeInfo.Platform,
		RuntimeType: run.RuntimeType(runtimeInfo.RuntimeType),
		Files:       make([]runtimeFileInfo, len(runtimeInfo.Files)),
	}
	for i, f := range runtimeInfo.Files {
		manifest.Files[i] = runtimeFileInfo(f)
	}

	return manifest
}
