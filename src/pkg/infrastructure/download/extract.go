package download

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"compress/zlib"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

// shouldIgnoreFile checks if a file should be ignored based on patterns
func shouldIgnoreFile(filename string, ignorePatterns []string) bool {
	if len(ignorePatterns) == 0 {
		return false
	}

	for _, pattern := range ignorePatterns {
		matched, err := filepath.Match(pattern, filepath.Base(filename))
		if err == nil && matched {
			return true
		}
		// Also try matching the full path
		matched, err = filepath.Match(pattern, filename)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// from https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
// nolint:gocyclo
func Untar(src, dst string, paths map[string]string) (files map[string]string, err error) {
	return UntarWithIgnore(src, dst, paths, nil)
}

// UntarWithIgnore is like Untar but accepts ignore patterns for files that should not be overwritten
func UntarWithIgnore(src, dst string, paths map[string]string, ignorePatterns []string) (files map[string]string, err error) {
	reader, err := os.Open(src)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open archive")
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil && err == nil {
			err = errClose
		}
	}()

	var tr *tar.Reader

	gz, err := gzip.NewReader(reader)
	if err != nil {
		var zl io.ReadCloser
		zl, err = zlib.NewReader(reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create new zlib reader after failed attempt at gzip")
		}
		defer func() {
			if errClose := zl.Close(); errClose != nil && err == nil {
				err = errClose
			}
		}()
		tr = tar.NewReader(zl)
	} else {
		defer func() {
			if errClose := gz.Close(); errClose != nil && err == nil {
				err = errClose
			}
		}()
		tr = tar.NewReader(gz)
	}

	files = make(map[string]string)
	var header *tar.Header
loop:
	for {
		header, err = tr.Next()
		switch {
		// if no more files are found return
		case err == io.EOF:
			err = nil
			break loop

		// return any other error
		case err != nil:
			break loop

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if header.Name == "" {
			continue
		}

		// path checking and dir extraction

		found, source, target := nameInPaths(header.Name, paths)
		if !found {
			continue
		}

		// if the target is not absolute, make relative to destination dir
		if !filepath.IsAbs(target) {
			target = filepath.Join(dst, target)
		}

		if header.FileInfo().IsDir() {
			err = os.MkdirAll(target, 0o700)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
		} else {
			targetDir := filepath.Dir(target)
			if !fs.Exists(targetDir) {
				err = os.MkdirAll(targetDir, 0o700)
				if err != nil {
					return nil, errors.Wrap(err, "failed to create target dir for file")
				}
			}

			if fs.Exists(target) && shouldIgnoreFile(target, ignorePatterns) {
				print.Verb("skipping existing file (matches ignore pattern):", target)
				continue
			}

			var file *os.File
			file, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return nil, errors.Wrap(err, "failed to open extract target file")
			}
			if _, err = io.Copy(file, tr); err != nil {
				_ = file.Close()
				return nil, errors.Wrap(err, "failed to copy archive file to destination")
			}
			if errClose := file.Close(); errClose != nil {
				return nil, errors.Wrap(errClose, "failed to close extract target file")
			}

			files[source] = target
		}
	}
	if err != nil {
		err = errors.Wrap(err, "unhandled error while parsing archive")
	}
	return files, err
}

// Unzip will un-compress a zip archive, moving all files and folders to an output directory.
// from: https://golangcode.com/unzip-files-in-go/
func Unzip(src, dst string, paths map[string]string) (files map[string]string, err error) {
	return UnzipWithIgnore(src, dst, paths, nil)
}

// UnzipWithIgnore is like Unzip but accepts ignore patterns for files that should not be overwritten
func UnzipWithIgnore(src, dst string, paths map[string]string, ignorePatterns []string) (files map[string]string, err error) {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil && err == nil {
			err = errClose
		}
	}()

	files = make(map[string]string)
	var archivedFile io.ReadCloser
	for _, header := range reader.File {
		if header.Name == "" {
			continue
		}

		// path checking and dir extraction
		found, source, target := nameInPaths(header.Name, paths)
		if !found {
			continue
		}

		// if the target is not absolute, make relative to destination dir
		if !filepath.IsAbs(target) {
			target = filepath.Join(dst, target)
		}

		if header.FileInfo().IsDir() {
			err = os.MkdirAll(target, 0o700)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
		} else {
			targetDir := filepath.Dir(target)
			if !fs.Exists(targetDir) {
				err = os.MkdirAll(targetDir, 0o700)
				if err != nil {
					return nil, errors.Wrap(err, "failed to create target dir for file")
				}
			}

			if fs.Exists(target) && shouldIgnoreFile(target, ignorePatterns) {
				print.Verb("skipping existing file (matches ignore pattern):", target)
				continue
			}

			archivedFile, err = header.Open()
			if err != nil {
				return nil, err
			}

			var file *os.File
			file, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, header.Mode())
			if err != nil {
				_ = archivedFile.Close()
				return nil, errors.Wrap(err, "failed to open extract target file")
			}

			_, err = io.Copy(file, archivedFile)
			if err != nil {
				_ = file.Close()
				_ = archivedFile.Close()
				return nil, errors.Wrap(err, "failed to copy archive file to destination")
			}
			if errClose := file.Close(); errClose != nil {
				_ = archivedFile.Close()
				return nil, errors.Wrap(errClose, "failed to close extract target file")
			}
			if errClose := archivedFile.Close(); errClose != nil {
				return nil, errors.Wrap(errClose, "failed to close archived file")
			}

			files[source] = target
		}
	}
	return files, err
}

// UnzipAllPreserveLayout extracts all files from a zip archive while preserving the
// archive layout. If all files are nested under a single top-level directory, that
// directory is stripped after extraction.
func UnzipAllPreserveLayout(src, dst string) (files map[string]string, err error) {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil && err == nil {
			err = errClose
		}
	}()

	files = make(map[string]string)
	for _, header := range reader.File {
		cleanName, ok := cleanArchivePath(header.Name)
		if !ok {
			continue
		}

		target := filepath.Join(dst, filepath.FromSlash(cleanName))
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
			continue
		}

		targetDir := filepath.Dir(target)
		if !fs.Exists(targetDir) {
			if err := os.MkdirAll(targetDir, 0o700); err != nil {
				return nil, errors.Wrap(err, "failed to create target dir for file")
			}
		}

		archivedFile, err := header.Open()
		if err != nil {
			return nil, err
		}

		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, header.Mode())
		if err != nil {
			_ = archivedFile.Close()
			return nil, errors.Wrap(err, "failed to open extract target file")
		}

		_, err = io.Copy(file, archivedFile)
		if err != nil {
			_ = file.Close()
			_ = archivedFile.Close()
			return nil, errors.Wrap(err, "failed to copy archive file to destination")
		}
		if errClose := file.Close(); errClose != nil {
			_ = archivedFile.Close()
			return nil, errors.Wrap(errClose, "failed to close extract target file")
		}
		if errClose := archivedFile.Close(); errClose != nil {
			return nil, errors.Wrap(errClose, "failed to close archived file")
		}

		files[cleanName] = target
	}

	return stripSingleTopLevelDir(dst, files)
}

// UntarAllPreserveLayout extracts all files from a tar archive while preserving the
// archive layout. If all files are nested under a single top-level directory, that
// directory is stripped after extraction.
func UntarAllPreserveLayout(src, dst string) (files map[string]string, err error) {
	reader, err := os.Open(src)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open archive")
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil && err == nil {
			err = errClose
		}
	}()

	tr, closer, err := newTarReader(reader)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closer != nil {
			if errClose := closer.Close(); errClose != nil && err == nil {
				err = errClose
			}
		}
	}()

	files = make(map[string]string)
	for {
		header, nextErr := tr.Next()
		switch {
		case nextErr == io.EOF:
			return stripSingleTopLevelDir(dst, files)
		case nextErr != nil:
			return nil, errors.Wrap(nextErr, "unhandled error while parsing archive")
		case header == nil:
			continue
		}

		cleanName, ok := cleanArchivePath(header.Name)
		if !ok {
			continue
		}

		target := filepath.Join(dst, filepath.FromSlash(cleanName))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
			continue
		case tar.TypeReg, tar.TypeRegA:
		default:
			continue
		}

		targetDir := filepath.Dir(target)
		if !fs.Exists(targetDir) {
			if err := os.MkdirAll(targetDir, 0o700); err != nil {
				return nil, errors.Wrap(err, "failed to create target dir for file")
			}
		}

		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return nil, errors.Wrap(err, "failed to open extract target file")
		}

		if _, err = io.Copy(file, tr); err != nil {
			_ = file.Close()
			return nil, errors.Wrap(err, "failed to copy archive file to destination")
		}
		if errClose := file.Close(); errClose != nil {
			return nil, errors.Wrap(errClose, "failed to close extract target file")
		}

		files[cleanName] = target
	}
}

func nameInPaths(name string, paths map[string]string) (found bool, source, target string) {
	for source, target = range paths {
		match, err := regexp.Compile(source)
		if err != nil {
			if name == source {
				found = true
				break
			}
		} else {
			if match.MatchString(name) {
				found = true
				break
			}
		}
	}
	if target == "" {
		target = filepath.Base(name)
	} else if strings.HasSuffix(target, "/") {
		target = filepath.Join(target, filepath.Base(name))
	}
	return
}

func newTarReader(reader io.Reader) (*tar.Reader, io.Closer, error) {
	gz, err := gzip.NewReader(reader)
	if err == nil {
		return tar.NewReader(gz), gz, nil
	}

	zl, zErr := zlib.NewReader(reader)
	if zErr != nil {
		return nil, nil, errors.Wrap(zErr, "failed to create new zlib reader after failed attempt at gzip")
	}

	return tar.NewReader(zl), zl, nil
}

func cleanArchivePath(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", false
	}

	cleaned := path.Clean(strings.TrimPrefix(strings.ReplaceAll(trimmed, `\\`, "/"), "/"))
	if cleaned == "." || cleaned == "" || cleaned == "/" {
		return "", false
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if isArchiveMetadataPath(cleaned) {
		return "", false
	}

	return cleaned, true
}

func isArchiveMetadataPath(cleaned string) bool {
	for _, part := range strings.Split(cleaned, "/") {
		if part == "__MACOSX" || strings.HasPrefix(part, "._") {
			return true
		}
	}

	return false
}

func stripSingleTopLevelDir(dst string, files map[string]string) (map[string]string, error) {
	if len(files) == 0 {
		return files, nil
	}

	var root string
	for _, target := range files {
		rel, err := filepath.Rel(dst, target)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 2 || parts[0] == "." || parts[0] == "" {
			return files, nil
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if parts[0] != root {
			return files, nil
		}
	}

	rootPath := filepath.Join(dst, root)
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		oldPath := filepath.Join(rootPath, entry.Name())
		newPath := filepath.Join(dst, entry.Name())
		if fs.Exists(newPath) {
			return nil, errors.Errorf("failed to strip archive root '%s': target already exists: %s", root, newPath)
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return nil, errors.Wrapf(err, "failed to move extracted path %s", oldPath)
		}
	}

	if err := os.Remove(rootPath); err != nil {
		return nil, errors.Wrapf(err, "failed to remove extracted root path %s", rootPath)
	}

	updated := make(map[string]string, len(files))
	for source, target := range files {
		rel, err := filepath.Rel(dst, target)
		if err != nil {
			return nil, err
		}
		prefix := root + string(filepath.Separator)
		if strings.HasPrefix(rel, prefix) {
			target = filepath.Join(dst, strings.TrimPrefix(rel, prefix))
		}
		updated[source] = target
	}

	ordered := make([]string, 0, len(updated))
	for source := range updated {
		ordered = append(ordered, source)
	}
	sort.Strings(ordered)

	result := make(map[string]string, len(updated))
	for _, source := range ordered {
		result[source] = updated[source]
	}

	return result, nil
}
