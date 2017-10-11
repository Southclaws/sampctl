package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/go-homedir"
	"github.com/pkg/errors"
)

// download.go handles downloading and extracting sa-mp server versions.
// Packages are cached in ~/.samp to avoid unnecessary downloads.

// GetCacheDir returns the full path to the user's cache directory
func GetCacheDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get home directory")
	}

	dir := filepath.Join(home, ".samp")
	return dir, os.MkdirAll(dir, 0755)
}

// FromCache first checks if a file is cached, then
func FromCache(cacheDir, filename, dir string, method func(string, string, []string) error, paths []string) (hit bool, err error) {
	path := filepath.Join(cacheDir, filename)

	if !exists(path) {
		hit = false
		return
	}

	err = method(path, dir, paths)
	if err != nil {
		hit = false
		err = errors.Wrapf(err, "failed to unzip package %s", path)
		return
	}

	return true, nil
}

// FromNet downloads the server package by filename from the specified endpoint to the cache dir
func FromNet(url, cacheDir, filename string) (result string, err error) {
	resp, err := http.Get(url)
	if err != nil {
		err = errors.Wrap(err, "failed to download package")
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			panic(err)
		}
	}()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "failed to read download contents")
		return
	}

	result = filepath.Join(cacheDir, filename)

	err = ioutil.WriteFile(result, content, 0655)
	if err != nil {
		err = errors.Wrap(err, "failed to write package to cache")
		return
	}

	return
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return true
}

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// from https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
func Untar(src, dst string, paths []string) (err error) {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	wantPath := func(path string) bool {
		for _, want := range paths {
			if path == want {
				return true
			}
		}
		return false
	}

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() {
		if err := gzr.Close(); err != nil {
			panic(err)
		}
	}()

	tr := tar.NewReader(gzr)
loop:
	for {
		header, err := tr.Next()
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

		var headerName string
		if strings.HasPrefix(header.Name, "samp03") {
			headerName = header.Name[7:]
		} else {
			headerName = header.Name
		}

		if !wantPath(headerName) {
			continue
		}

		// the target location where the dir/file should be created - trimming off "samp03"
		target := filepath.Join(dst, headerName)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		if header.Typeflag == tar.TypeReg {
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
	if err != nil {
		return
	}
	return createDirs(dst)
}

// Unzip will un-compress a zip archive, moving all files and folders to an output directory.
// from: https://golangcode.com/unzip-files-in-go/
func Unzip(src, dest string, paths []string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	wantPath := func(path string) bool {
		for _, want := range paths {
			if path == want {
				return true
			}
		}
		return false
	}

	for _, f := range r.File {
		if !wantPath(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		fpath := filepath.Join(dest, f.Name)

		if !f.FileInfo().IsDir() {
			err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
			if err != nil {
				return err
			}

			f, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
	}
	return createDirs(dest)
}

// createDirs simply creates the necessary gamemodes and filterscripts directories
func createDirs(dir string) (err error) {
	err = os.MkdirAll(filepath.Join(dir, "gamemodes"), 0755)
	if err != nil {
		return
	}
	err = os.MkdirAll(filepath.Join(dir, "filterscripts"), 0755)
	return
}
