package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
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

// GetServerPackage checks if a cached package is available and if not, downloads it to dir
func GetServerPackage(endpoint, version, dir string) (err error) {
	fmt.Printf("Downloading package %s from endpoint %s into %s\n", version, endpoint, dir)

	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	hit, err := serverFromCache(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = serverFromNet(endpoint, cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(version, dir string) (err error) {
	fmt.Printf("Downloading compiler package\n")

	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	hit, err := compilerFromCache(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = compilerFromNet(cacheDir, version, dir)
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

	dir := filepath.Join(home, ".samp")
	return dir, os.MkdirAll(dir, 0755)
}

func serverFromCache(cacheDir, version, dir string) (hit bool, err error) {
	var filename string
	var method func(string, string) error

	pkg, ok := Packages[version]
	if !ok {
		return false, errors.Errorf("invalid version '%s'", version)
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

	hit, err = fromCache(cacheDir, filename, dir, method)
	if !hit {
		return
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		return false, errors.Errorf("validation errors: %#v", errs)
	}

	return true, nil
}

func compilerFromCache(cacheDir, version, dir string) (hit bool, err error) {
	var (
		filename string
		method   func(string, string) error
	)

	if runtime.GOOS == "windows" {
		_, filename = compilerURL(pawnWin32, version)
		method = Unzip
	} else if runtime.GOOS == "linux" {
		_, filename = compilerURL(pawnLinux, version)
		method = Untar
	} else if runtime.GOOS == "darwin" {
		_, filename = compilerURL(pawnMacOS, version)
		method = Unzip
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	hit, err = fromCache(cacheDir, filename, dir, method)
	if !hit {
		return false, nil
	}

	return
}

// fromCache first checks if a file is cached, then
func fromCache(cacheDir, filename, dir string, method func(string, string) error) (hit bool, err error) {
	fullPath := filepath.Join(cacheDir, filename)

	if !exists(fullPath) {
		hit = false
		return
	}

	err = method(fullPath, dir)
	if err != nil {
		hit = false
		err = errors.Wrapf(err, "failed to unzip package %s", fullPath)
	}

	return
}

// serverFromNet downloads a server package to the cache, then calls fromCache to finish the job
func serverFromNet(endpoint, cacheDir, version, dir string) (err error) {
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

	if !exists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !exists(cacheDir) {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		err = errors.Wrapf(err, "failed to parse endpoint %s", endpoint)
		return
	}
	u.Path = path.Join(u.Path, filename)

	fullPath, err := fromNet(u.String(), cacheDir, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	err = method(fullPath, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		return errors.Errorf("validation errors: %v", errs)
	}

	return
}

func compilerFromNet(cacheDir, version, dir string) (err error) {
	var (
		rawurl   string
		filename string
		method   func(string, string) error
	)

	if runtime.GOOS == "windows" {
		rawurl, filename = compilerURL(pawnWin32, version)
		method = Unzip
	} else if runtime.GOOS == "linux" {
		rawurl, filename = compilerURL(pawnLinux, version)
		method = Untar
	} else if runtime.GOOS == "darwin" {
		rawurl, filename = compilerURL(pawnMacOS, version)
		method = Unzip
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	if !exists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !exists(cacheDir) {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	path, err := fromNet(rawurl, cacheDir, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	err = method(path, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", path)
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		return errors.Errorf("validation errors: %v", errs)
	}

	return
}

// fromNet downloads the server package by filename from the specified endpoint
func fromNet(url, cacheDir, filename string) (result string, err error) {
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

	err = ioutil.WriteFile(filename, content, 0655)
	if err != nil {
		err = errors.Wrap(err, "failed to write package to cache")
		return
	}

	return
}

// ValidateServerDir ensures the dir has all the necessary files to run a server, it also performs an MD5
// checksum against the binary to prevent running anything unwanted.
func ValidateServerDir(dir, version string) (errs []error) {
	if !exists(filepath.Join(dir, getNpcBinary())) {
		errs = append(errs, errors.New("missing npc binary"))
	}
	if !exists(filepath.Join(dir, getAnnounceBinary())) {
		errs = append(errs, errors.New("missing announce binary"))
	}
	if !exists(filepath.Join(dir, getServerBinary())) {
		errs = append(errs, errors.New("missing server binary"))
	} else {
		// now perform an md5 on the server
		ok, err := matchesChecksum(filepath.Join(dir, getServerBinary()), version)
		if err != nil {
			errs = append(errs, errors.New("failed to match checksum"))
		} else if !ok {
			errs = append(errs, errors.Errorf("existing binary does not match checksum for version %s", version))
		}
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
func createDirs(dir string) (err error) {
	err = os.MkdirAll(filepath.Join(dir, "gamemodes"), 0755)
	if err != nil {
		return
	}
	err = os.MkdirAll(filepath.Join(dir, "filterscripts"), 0755)
	return
}
