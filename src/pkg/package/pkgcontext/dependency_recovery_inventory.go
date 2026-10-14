package pkgcontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func validRecoveryHash(hash string) bool {
	data, err := hex.DecodeString(hash)
	return err == nil && len(data) == sha256.Size
}

func recoverySnapshotHash(file dependencyRecoveryFile) string {
	data := append([]byte(fmt.Sprintf("%s:%d:%t:", file.Path, file.Mode, file.Exists)), file.Data...)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func recoveryCleanupEntries(ctx context.Context, root string) ([]dependencyRecoveryCleanupEntry, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("cleanup path %s must be a directory", root)
	}

	var entries []dependencyRecoveryCleanupEntry
	var buffer []byte
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if path == root {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		file := dependencyRecoveryCleanupEntry{Path: filepath.ToSlash(relative), Mode: uint32(info.Mode())}
		switch {
		case info.IsDir():
		case info.Mode()&os.ModeSymlink != 0:
			file.Link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		case info.Mode().IsRegular():
			f, err := os.Open(path)
			if err != nil {
				return err
			}

			hash := sha256.New()
			if buffer == nil {
				buffer = make([]byte, 32*1024)
			}
			_, readErr := io.CopyBuffer(hash, dependencyContextReader{ctx: ctx, reader: f}, buffer)
			if err := errors.Join(readErr, f.Close()); err != nil {
				return err
			}
			file.Hash = hex.EncodeToString(hash.Sum(nil))
		default:
			return fmt.Errorf("unsupported staged entry %s", path)
		}
		entries = append(entries, file)
		return nil
	})

	return entries, errors.Join(err, ctx.Err())
}

func recoveryTreeHash(ctx context.Context, path string, syncFiles bool) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("dependency recovery directory %s must be a directory", path)
	}

	hash := sha256.New()
	var directories []string
	var buffer []byte
	err = filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(path, entryPath)
		if err != nil {
			return err
		}

		size := info.Size()
		if info.IsDir() {
			size = 0
		}
		if _, err := fmt.Fprintf(hash, "%q:%d:%d:", filepath.ToSlash(relative), info.Mode(), size); err != nil {
			return err
		}

		switch {
		case info.IsDir():
			directories = append(directories, entryPath)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(entryPath)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(hash, "%q", target); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			file, err := os.Open(entryPath)
			if err != nil {
				return err
			}

			if buffer == nil {
				buffer = make([]byte, 32*1024)
			}
			_, readErr := io.CopyBuffer(hash, dependencyContextReader{ctx: ctx, reader: file}, buffer)
			var syncErr error
			if syncFiles && readErr == nil {
				syncErr = syncDependencyRecoveryFile(file, info)
			}
			if err := errors.Join(readErr, syncErr, file.Close()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported dependency entry %s", entryPath)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	if syncFiles {
		for i := len(directories) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if err := fs.SyncDir(directories[i]); err != nil {
				return "", err
			}
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
