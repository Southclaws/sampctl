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
	"strings"

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

	if !exists(filename) {
		return false, nil
	}

	err = method(filename, cwd)
	if err != nil {
		return false, errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	empty, errs := validate(cwd, version)
	if errs != nil {
		return false, errors.Errorf("validation errors: %#v", errs)
	}
	if empty {
		return false, errors.Errorf("dir is empty")
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

	if !exists(cwd) {
		err := os.MkdirAll(cwd, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", cwd)
		}
	}

	if !exists(cacheDir) {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	content, err := downloadPackage(endpoint, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	fullPath := filepath.Join(cacheDir, filename)

	err = ioutil.WriteFile(fullPath, content, 0655)
	if err != nil {
		return errors.Wrap(err, "failed to write package to cache")
	}

	err = method(fullPath, cwd)
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	empty, errs := validate(cwd, version)
	if errs != nil {
		return errors.Errorf("validation errors: %v", errs)
	}
	if empty {
		return errors.Errorf("dir is empty")
	}

	return
}

// downloadPackage downloads the server package by filename from the specified endpoint
func downloadPackage(endpoint, filename string) (content []byte, err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		err = errors.Wrapf(err, "failed to parse endpoint %s", endpoint)
		return
	}
	u.Path = path.Join(u.Path, filename)

	resp, err := http.Get(u.String())
	if err != nil {
		err = errors.Wrap(err, "failed to download package")
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			panic(err)
		}
	}()

	return ioutil.ReadAll(resp.Body)
}

// validate ensures the cwd has all the necessary files to run a server, it also performs an MD5
// checksum against the binary to prevent running anything unwanted.
func validate(cwd, version string) (empty bool, errs []error) {
	missing := 0
	if !exists(filepath.Join(cwd, getNpcBinary())) {
		errs = append(errs, errors.New("missing npc binary"))
		missing++
	}
	if !exists(filepath.Join(cwd, getAnnounceBinary())) {
		errs = append(errs, errors.New("missing announce binary"))
		missing++
	}
	if !exists(filepath.Join(cwd, getServerBinary())) {
		errs = append(errs, errors.New("missing server binary"))
		missing++
	} else {
		// now perform an md5 on the server
		ok, err := matchesChecksum(filepath.Join(cwd, getServerBinary()), version)
		if err != nil {
			errs = append(errs, errors.New("failed to match checksum"))
		} else if !ok {
			errs = append(errs, errors.Errorf("existing binary does not match checksum for version %s", version))
		}
	}

	if !exists(filepath.Join(cwd, "gamemodes")) {
		errs = append(errs, errors.New("missing gamemodes dir"))
		missing++
	}
	if !exists(filepath.Join(cwd, "filterscripts")) {
		errs = append(errs, errors.New("missing gamemodes dir"))
		missing++
	}

	if missing == 3 {
		empty = true
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
func Untar(src, dst string) (err error) {
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

		if !isBinary(headerName) {
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
		if !isBinary(f.Name) {
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
func createDirs(cwd string) (err error) {
	err = os.MkdirAll(filepath.Join(cwd, "gamemodes"), 0755)
	if err != nil {
		return
	}
	err = os.MkdirAll(filepath.Join(cwd, "filterscripts"), 0755)
	return
}
