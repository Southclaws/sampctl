package server

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/util"
)

// GetServerPackage checks if a cached package is available and if not, downloads it to dir
func GetServerPackage(endpoint, version, dir string) (err error) {
	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return err
	}

	hit, err := FromCache(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = FromNet(endpoint, cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// FromCache tries to grab a server package from cache, `hit` indicates if it was successful
func FromCache(cacheDir, version, dir string) (hit bool, err error) {
	var (
		filename string
		method   download.ExtractFunc
		paths    map[string]string
	)

	pkg, ok := Packages[version]
	if !ok {
		return false, errors.Errorf("invalid version '%s'", version)
	}

	if runtime.GOOS == "windows" {
		filename = pkg.Win32
		method = download.Unzip
		paths = pkg.Win32Paths
	} else if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		filename = pkg.Linux
		method = download.Untar
		paths = pkg.LinuxPaths
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	hit, err = download.FromCache(cacheDir, filename, dir, method, paths)
	if !hit || err != nil {
		return
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		return false, errors.Errorf("validation errors: %#v", errs)
	}

	fmt.Printf("Using cached package for %s\n", version)

	return true, nil
}

// FromNet downloads a server package to the cache, then calls FromCache to finish the job
func FromNet(endpoint, cacheDir, version, dir string) (err error) {
	fmt.Printf("Downloading package %s from endpoint %s into %s\n", version, endpoint, dir)

	var (
		filename string
		method   download.ExtractFunc
		paths    map[string]string
	)

	pkg, ok := Packages[version]
	if !ok {
		return errors.Errorf("invalid version '%s'", version)
	}

	if runtime.GOOS == "windows" {
		filename = pkg.Win32
		method = download.Unzip
		paths = pkg.Win32Paths
	} else if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		filename = pkg.Linux
		method = download.Untar
		paths = pkg.LinuxPaths
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	if !util.Exists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !util.Exists(cacheDir) {
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

	fullPath, err := download.FromNet(u.String(), cacheDir, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	err = method(fullPath, dir, paths)
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
	if !util.Exists(filepath.Join(dir, getNpcBinary())) {
		errs = append(errs, errors.New("missing npc binary"))
	}
	if !util.Exists(filepath.Join(dir, getAnnounceBinary())) {
		errs = append(errs, errors.New("missing announce binary"))
	}
	if !util.Exists(filepath.Join(dir, getServerBinary())) {
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

// CreateServerDirectories simply creates the necessary gamemodes and filterscripts directories
func CreateServerDirectories(dir string) (err error) {
	err = os.MkdirAll(filepath.Join(dir, "gamemodes"), 0755)
	if err != nil {
		return
	}
	err = os.MkdirAll(filepath.Join(dir, "filterscripts"), 0755)
	return
}
