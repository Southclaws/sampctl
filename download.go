package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"log"
	"os"
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
	if runtime.GOOS == "windows" {
		filename = filepath.Join(cacheDir, Packages[version].Win32)

		_, err = os.Stat(filename)
		if os.IsNotExist(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "failed to check cached package existence")
		}

		_, err = Unzip(filename, cwd)
		if err != nil {
			return false, errors.Wrapf(err, "failed to unzip package %s", filename)
		}
	} else if runtime.GOOS == "linux" {
		filename = filepath.Join(cacheDir, Packages[version].Linux)

		_, err = os.Stat(filename)
		if os.IsNotExist(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "failed to check cached package existence")
		}

		reader, err := os.Open(filename)
		if err != nil {
			return false, errors.Wrapf(err, "failed to open cached package %s", filename)
		}
		err = Untar(cwd, reader)
		if err != nil {
			return false, errors.Wrapf(err, "failed to untar package %s", filename)
		}
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	return true, nil
}

// fromNet downloads a server package to the cache, then calls fromCache to finish the job
func fromNet(endpoint, cacheDir, version, cwd string) (err error) {
	return
}

// cleanUp removes unnecessary files and folders from the extracted package such as readmes etc.
func cleanUp(cwd string) (err error) {
	return
}

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
// from https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
func Untar(dst string, r io.Reader) error {

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
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer f.Close()

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
}

// Unzip will un-compress a zip archive, moving all files and folders to an output directory.
// from: https://golangcode.com/unzip-files-in-go/
func Unzip(src, dest string) ([]string, error) {
	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}
		defer rc.Close()

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)
		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
		} else {
			// Make File
			var fdir string
			if lastIndex := strings.LastIndex(fpath, string(os.PathSeparator)); lastIndex > -1 {
				fdir = fpath[:lastIndex]
			}

			err = os.MkdirAll(fdir, os.ModePerm)
			if err != nil {
				log.Fatal(err)
				return filenames, err
			}
			f, err := os.OpenFile(
				fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return filenames, err
			}
			defer f.Close()

			_, err = io.Copy(f, rc)
			if err != nil {
				return filenames, err
			}
		}
	}
	return filenames, nil
}
