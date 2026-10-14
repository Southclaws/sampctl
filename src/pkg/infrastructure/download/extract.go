package download

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"compress/zlib"
	"io"
	"os"
	"path/filepath"
	"regexp"
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

// Untar extracts selected files from a gzip or zlib tar archive.
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
		if _, seekErr := reader.Seek(0, io.SeekStart); seekErr != nil {
			return nil, errors.Wrap(seekErr, "failed to reset archive reader after failed gzip probe")
		}

		var zl io.ReadCloser
		zl, err = zlib.NewReader(reader)
		if err != nil {
			return nil, errors.Wrap(err, "archive could not be read as gzip or zlib")
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
		case err == io.EOF:
			err = nil
			break loop

		case err != nil:
			break loop

		case header == nil:
			continue
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if header.Name == "" {
			continue
		}

		found, source, target := nameInPaths(header.Name, paths)
		if !found {
			continue
		}

		if !filepath.IsAbs(target) {
			target = filepath.Join(dst, target)
		}

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

		if err := writeExtractedFile(target, os.FileMode(header.Mode), tr); err != nil {
			return nil, err
		}

		files[source] = target
	}

	if err != nil {
		err = errors.Wrap(err, "failed to read archive")
	}

	return files, err
}

// Unzip extracts selected files from a zip archive.
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

		found, source, target := nameInPaths(header.Name, paths)
		if !found {
			continue
		}

		if !filepath.IsAbs(target) {
			target = filepath.Join(dst, target)
		}

		if header.FileInfo().IsDir() {
			err = os.MkdirAll(target, 0o700)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
			continue
		}

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

		copyErr := writeExtractedFile(target, header.Mode(), archivedFile)
		closeErr := archivedFile.Close()
		if copyErr != nil {
			if closeErr != nil {
				print.Warn("failed to close archived file after extraction error:", closeErr)
			}

			return nil, copyErr
		}

		if closeErr != nil {
			return nil, errors.Wrap(closeErr, "failed to close archived file")
		}

		files[source] = target
	}

	return files, err
}

func writeExtractedFile(target string, mode os.FileMode, reader io.Reader) (err error) {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return errors.Wrap(err, "failed to open extract target file")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if err != nil {
				print.Warn("failed to close extract target file after copy error:", closeErr)
			} else {
				err = errors.Wrap(closeErr, "failed to close extract target file")
			}
		}
	}()

	if _, err := io.Copy(file, reader); err != nil {
		return errors.Wrap(err, "failed to copy archive file to destination")
	}

	return nil
}

func nameInPaths(name string, paths map[string]string) (found bool, source, target string) {
	for source, target = range paths {
		match, err := regexp.Compile(source)
		if err != nil {
			if name == source {
				found = true
				break
			}
		} else if match.MatchString(name) {
			found = true
			break
		}
	}

	if target == "" {
		target = filepath.Base(name)
	} else if strings.HasSuffix(target, "/") {
		target = filepath.Join(target, filepath.Base(name))
	}

	return
}
