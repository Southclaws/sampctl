package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/minio/go-homedir"
	"github.com/pkg/errors"
)

// download.go handles downloading and extracting sa-mp server versions.
// Packages are cached in ~/.samp to avoid unnecessary downloads.

// GetPackage checks if a cached package is available and if not, downloads it to the cwd
func GetPackage(endpoint, version, cwd string) (err error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	hit, err := fromCache(cacheDir, version, cwd)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = fromNet(endpoint, cacheDir, version, cwd)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	err = cleanUp(cwd)
	if err != nil {
		return errors.Wrapf(err, "failed to clean up extracted package %s", version)
	}

	return
}

// getCacheDir returns the full path to the user's cache directory
func getCacheDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get home directory")
	}

	return filepath.Join(home, ".samp"), nil
}

func fromCache(cacheDir, version, cwd string) (hit bool, err error) {
	var filename string
	var method func(string, string) error

	pkg, ok := Packages[version]
	if !ok {
		return false, errors.Errorf("invalid version '%s'", version)
	}

	if runtime.GOOS == "windows" {
		filename = filepath.Join(cacheDir, pkg.Win32)
		method = Unzip
	} else if runtime.GOOS == "linux" {
		filename = filepath.Join(cacheDir, pkg.Linux)
		method = Untar
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "failed to check cached package existence")
	}

	err = method(filename, cwd)
	if err != nil {
		return false, errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	return true, nil
}

// fromNet downloads a server package to the cache, then calls fromCache to finish the job
func fromNet(endpoint, cacheDir, version, cwd string) (err error) {
	var filename string
	var method func(string, string) error

	pkg, ok := Packages[version]
	if !ok {
		return errors.Errorf("invalid version '%s'", version)
	}

	if runtime.GOOS == "windows" {
		filename = pkg.Win32
		method = Unzip
	} else if runtime.GOOS == "linux" {
		filename = pkg.Linux
		method = Untar
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.Wrapf(err, "failed to parse endpoint %s", endpoint)
	}
	u.Path = path.Join(u.Path, filename)

	resp, err := http.Get(u.String())
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			panic(err)
		}
	}()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	fullPath := filepath.Join(cacheDir, filename)

	err = ioutil.WriteFile(fullPath, content, 0655)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	err = method(fullPath, cwd)
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	return
}

// cleanUp removes unnecessary files and folders from the extracted package such as readmes etc.
func cleanUp(cwd string) (err error) {
	return
}

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// from https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
func Untar(src, dst string) error {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

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
	for {
		header, err := tr.Next()
		switch {
		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0655); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
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
}

// Unzip will un-compress a zip archive, moving all files and folders to an output directory.
// from: https://golangcode.com/unzip-files-in-go/
func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	for _, f := range r.File {

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			// Make Folder
			err = os.MkdirAll(fpath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			// Make File
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
	return nil
}
