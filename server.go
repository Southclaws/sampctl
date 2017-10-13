package main

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// GetServerPackage checks if a cached package is available and if not, downloads it to dir
func GetServerPackage(endpoint, version, dir string) (err error) {
	fmt.Printf("Downloading package %s from endpoint %s into %s\n", version, endpoint, dir)

	cacheDir, err := GetCacheDir()
	if err != nil {
		return err
	}

	hit, err := ServerFromCache(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = ServerFromNet(endpoint, cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// ServerFromCache tries to grab a server package from cache, `hit` indicates if it was successful
func ServerFromCache(cacheDir, version, dir string) (hit bool, err error) {
	var filename string
	var method ExtractFunc

	pkg, ok := Packages[version]
	if !ok {
		return false, errors.Errorf("invalid version '%s'", version)
	}

	if runtime.GOOS == "windows" {
		filename = pkg.Win32
		method = Unzip
	} else if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		filename = pkg.Linux
		method = Untar
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	hit, err = FromCache(cacheDir, filename, dir, method, map[string]string{
		getServerBinary():   filepath.Join(cacheDir, getServerBinary()),
		getAnnounceBinary(): filepath.Join(cacheDir, getAnnounceBinary()),
		getNpcBinary():      filepath.Join(cacheDir, getNpcBinary()),
	})
	if !hit || err != nil {
		return
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		return false, errors.Errorf("validation errors: %#v", errs)
	}

	return true, nil
}

// ServerFromNet downloads a server package to the cache, then calls FromCache to finish the job
func ServerFromNet(endpoint, cacheDir, version, dir string) (err error) {
	var filename string
	var method ExtractFunc

	pkg, ok := Packages[version]
	if !ok {
		return errors.Errorf("invalid version '%s'", version)
	}

	if runtime.GOOS == "windows" {
		filename = pkg.Win32
		method = Unzip
	} else if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
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

	fullPath, err := FromNet(u.String(), cacheDir, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	err = method(fullPath, dir, map[string]string{
		getServerBinary():   filepath.Join(cacheDir, getServerBinary()),
		getAnnounceBinary(): filepath.Join(cacheDir, getAnnounceBinary()),
		getNpcBinary():      filepath.Join(cacheDir, getNpcBinary()),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		return errors.Errorf("validation errors: %v", errs)
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
