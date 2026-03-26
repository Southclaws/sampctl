package fs

import (
	"encoding/json"
	stderrors "errors"
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
	if err := f.Chmod(PermFileTemp); err != nil {
		return stderrors.Join(err, cleanupTempFile(f, tmp))
	}
	if _, err := f.Write(data); err != nil {
		return stderrors.Join(err, cleanupTempFile(f, tmp))
	}
	if err := f.Sync(); err != nil {
		return stderrors.Join(err, cleanupTempFile(f, tmp))
	}
	if err := f.Close(); err != nil {
		return stderrors.Join(err, removeFileIfExists(tmp))
	}
	if err := os.Rename(tmp, path); err != nil {
		return stderrors.Join(err, removeFileIfExists(tmp))
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
	if err := f.Chmod(PermFileTemp); err != nil {
		return stderrors.Join(err, cleanupTempFile(f, tmp))
	}
	if _, err := io.Copy(f, r); err != nil {
		return stderrors.Join(err, cleanupTempFile(f, tmp))
	}
	if err := f.Sync(); err != nil {
		return stderrors.Join(err, cleanupTempFile(f, tmp))
	}
	if err := f.Close(); err != nil {
		return stderrors.Join(err, removeFileIfExists(tmp))
	}
	if err := os.Rename(tmp, path); err != nil {
		return stderrors.Join(err, removeFileIfExists(tmp))
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

func cleanupTempFile(f *os.File, path string) error {
	return stderrors.Join(closeTempFile(f), removeFileIfExists(path))
}

func closeTempFile(f *os.File) error {
	if f == nil {
		return nil
	}

	err := f.Close()
	if err != nil && !stderrors.Is(err, os.ErrClosed) {
		return err
	}

	return nil
}

func removeFileIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
