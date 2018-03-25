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

	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
)

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// from https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
// nolint:gocyclo
func Untar(src, dst string, paths map[string]string) (files map[string]string, err error) {
	reader, err := os.Open(src)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open archive")
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil {
			panic(errClose)
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
			if err = zl.Close(); err != nil {
				panic(err)
			}
		}()
		tr = tar.NewReader(zl)
	} else {
		defer func() {
			if err = gz.Close(); err != nil {
				panic(err)
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
			err = os.MkdirAll(target, 0700)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
		} else {
			targetDir := filepath.Dir(target)
			if !util.Exists(targetDir) {
				err = os.MkdirAll(targetDir, 0700)
				if err != nil {
					return nil, errors.Wrap(err, "failed to create target dir for file")
				}
			}

			var file *os.File
			file, err = os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return nil, errors.Wrap(err, "failed to open extract target file")
			}
			defer func() {
				if err = file.Close(); err != nil {
					panic(err)
				}
			}()

			if _, err = io.Copy(file, tr); err != nil {
				return nil, errors.Wrap(err, "failed to copy archive file to destination")
			}

			files[source] = target
		}
	}
	if err != nil {
		err = errors.Wrap(err, "unhandled error while parsing archive")
	}
	return
}

// Unzip will un-compress a zip archive, moving all files and folders to an output directory.
// from: https://golangcode.com/unzip-files-in-go/
func Unzip(src, dst string, paths map[string]string) (files map[string]string, err error) {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errClose := reader.Close(); errClose != nil {
			panic(errClose)
		}
	}()

	files = make(map[string]string)
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
			err = os.MkdirAll(target, 0700)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create dir for target")
			}
		} else {
			targetDir := filepath.Dir(target)
			if !util.Exists(targetDir) {
				err = os.MkdirAll(targetDir, os.ModePerm)
				if err != nil {
					return nil, errors.Wrap(err, "failed to create target dir for file")
				}
			}

			archivedFile, err := header.Open()
			if err != nil {
				return nil, err
			}
			defer func() {
				if err = archivedFile.Close(); err != nil {
					panic(err)
				}
			}()

			var file *os.File
			file, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, header.Mode())
			if err != nil {
				return nil, errors.Wrap(err, "failed to open extract target file")
			}
			defer func() {
				if err = file.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(file, archivedFile)
			if err != nil {
				return nil, errors.Wrap(err, "failed to copy archive file to destination")
			}

			files[source] = target
		}
	}
	return
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
