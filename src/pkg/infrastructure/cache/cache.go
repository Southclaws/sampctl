package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func Path(cacheDir string, parts ...string) string {
	all := append([]string{cacheDir}, parts...)
	return filepath.Join(all...)
}

func EnsureDir(dir string, perm os.FileMode) error {
	return os.MkdirAll(dir, perm)
}

func EnsureDirForFile(path string, perm os.FileMode) error {
	return EnsureDir(filepath.Dir(path), perm)
}

func WriteFileAtomic(path string, data []byte, dirPerm, filePerm os.FileMode) error {
	if err := EnsureDirForFile(path, dirPerm); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	f, err := os.CreateTemp(dir, "."+base+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = os.Remove(tmp)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(path, filePerm); err != nil {
		return err
	}
	return nil
}

func WriteFromReaderAtomic(path string, r io.Reader, dirPerm, filePerm os.FileMode) error {
	if err := EnsureDirForFile(path, dirPerm); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	f, err := os.CreateTemp(dir, "."+base+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = os.Remove(tmp)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(path, filePerm); err != nil {
		return err
	}
	return nil
}

func WriteJSONAtomic(path string, v any, dirPerm, filePerm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}
	data = append(data, '\n')
	return WriteFileAtomic(path, data, dirPerm, filePerm)
}

func IsFresh(path string, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < ttl
}

func ReadJSON[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func GetOrRefreshJSON[T any](
	ctx context.Context,
	path string,
	ttl time.Duration,
	dirPerm, filePerm os.FileMode,
	fetch func(context.Context) (T, error),
) (value T, refreshed bool, err error) {
	if IsFresh(path, ttl) {
		value, err = ReadJSON[T](path)
		return value, false, err
	}

	value, err = fetch(ctx)
	if err != nil {
		return value, false, err
	}
	if err := WriteJSONAtomic(path, value, dirPerm, filePerm); err != nil {
		return value, false, err
	}
	return value, true, nil
}
