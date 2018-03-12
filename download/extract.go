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

	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
)

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// from https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
// nolint:gocyclo
func Untar(src, dst string, paths map[string]string) (err error) {
	r, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "failed to open archive")
	}
	defer func() {
		if errClose := r.Close(); errClose != nil {
			panic(errClose)
		}
	}()

	var tr *tar.Reader

	gz, err := gzip.NewReader(r)
	if err != nil {
		var zl io.ReadCloser
		zl, err = zlib.NewReader(r)
		if err != nil {
			return errors.Wrap(err, "failed to create new zlib reader after failed attempt at gzip")
		}
		defer func() {
			if err = zl.Close(); err != nil {
				return
			}
		}()
		tr = tar.NewReader(zl)
	} else {
		defer func() {
			if err = gz.Close(); err != nil {
				return
			}
		}()
		tr = tar.NewReader(gz)
	}

	var header *tar.Header
loop:
	for {
		header, err = tr.Next()
		switch {
		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			break loop

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// search by regular expression

		var (
			source string
			target string
			match  *regexp.Regexp
			found  bool
		)

		for source, target = range paths {
			match, err = regexp.Compile(source)
			if err != nil {
				if header.Name == source {
					found = true
					break
				}
			} else {
				if match.MatchString(header.Name) {
					found = true
					if target == "" {
						target = header.Name
					}
					break
				}
			}
		}

		if !found {
			continue
		}

		// if the target is not absolute, make relative to destination dir
		if !filepath.IsAbs(target) {
			target = filepath.Join(dst, target)
		}

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if header.FileInfo().IsDir() {
			err = os.MkdirAll(target, 0700)
			if err != nil {
				return errors.Wrap(err, "failed to create dir for target")
			}
		} else {
			targetDir := filepath.Dir(target)
			if !util.Exists(targetDir) {
				err = os.MkdirAll(targetDir, 0700)
				if err != nil {
					return errors.Wrap(err, "failed to create target dir for file")
				}
			}

			var f *os.File
			f, err = os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return errors.Wrap(err, "failed to open extract target file")
			}
			defer func() {
				if err = f.Close(); err != nil {
					return
				}
			}()

			// copy over contents
			if _, err = io.Copy(f, tr); err != nil {
				return errors.Wrap(err, "failed to copy contents to extract target file")
			}
		}
	}
	if err != nil {
		err = errors.Wrap(err, "unhandled error while parsing archive")
	}
	return
}

// Unzip will un-compress a zip archive, moving all files and folders to an output directory.
// from: https://golangcode.com/unzip-files-in-go/
func Unzip(src, dst string, paths map[string]string) (err error) {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if errClose := r.Close(); errClose != nil {
			panic(errClose)
		}
	}()

	for _, f := range r.File {
		var (
			source string
			target string
			match  *regexp.Regexp
			found  bool
		)

		for source, target = range paths {
			match, err = regexp.Compile(source)
			if err != nil {
				return
			}

			if match.MatchString(f.Name) {
				found = true
				break
			}
		}

		if !found {
			continue
		}

		// if the target is not absolute, make relative to destination dir
		if !filepath.IsAbs(target) {
			target = filepath.Join(dst, target)
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err = rc.Close(); err != nil {
				return
			}
		}()

		if !f.FileInfo().IsDir() {
			err = os.MkdirAll(filepath.Dir(target), os.ModePerm)
			if err != nil {
				return err
			}

			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err = f.Close(); err != nil {
					return
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
	}
	return
}
