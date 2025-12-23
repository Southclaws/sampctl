package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

const (
	runtimeManifestDirName  = ".sampctl"
	runtimeManifestFileName = "sampctl-runtime-manifest.json"
	runtimeStagingDir       = "runtime_staging"
)

var runtimeManifestRelativePath = filepath.Join(runtimeManifestDirName, runtimeManifestFileName)

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
	manifestDir := filepath.ToSlash(runtimeManifestDirName)

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
			if relPath != "" && strings.EqualFold(relPath, manifestDir) {
				return iofs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(relPath, manifestRel) {
			return nil
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

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	h := sha256.New()
	size, err := io.Copy(h, file)
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
	return os.WriteFile(path, data, 0o600)
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
	for _, file := range manifest.Files {
		fullPath := filepath.Join(root, filepath.FromSlash(file.Path))
		info, err := os.Stat(fullPath)
		if err != nil {
			return errors.Wrapf(err, "missing runtime file %s", file.Path)
		}
		if info.Size() != file.Size {
			return errors.Errorf("runtime file %s size mismatch", file.Path)
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

func copyRuntimeFiles(manifest runtimeManifest, srcRoot, destRoot string) error {
	for _, file := range manifest.Files {
		src := filepath.Join(srcRoot, filepath.FromSlash(file.Path))
		dest := filepath.Join(destRoot, filepath.FromSlash(file.Path))

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return errors.Wrap(err, "failed to create runtime destination directory")
		}

		if err := copyFileWithMode(src, dest, os.FileMode(file.Mode)); err != nil {
			return errors.Wrapf(err, "failed to copy runtime file %s", file.Path)
		}
	}
	return nil
}

func copyFileWithMode(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func removeRuntimeFiles(manifest runtimeManifest, root string) error {
	dirSet := map[string]struct{}{}
	for _, file := range manifest.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
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
		_ = os.Remove(dir)
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

func runtimeManifestPath(root string) string {
	return filepath.Join(root, runtimeManifestRelativePath)
}
