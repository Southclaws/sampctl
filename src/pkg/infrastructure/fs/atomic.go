package fs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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
	if err := f.Chmod(PermFileTemp); err != nil {
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
	if err := f.Chmod(PermFileTemp); err != nil {
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
