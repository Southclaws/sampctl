package runtime

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// GetServerPackage checks if a cached package is available and if not, downloads it to dir
func GetServerPackage(version, dir, platform string) (err error) {
	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return errors.Wrap(err, "failed to get or create cache directory")
	}

	hit, err := FromCache(cacheDir, version, dir, platform)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = FromNet(cacheDir, version, dir, platform)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// FromCache tries to grab a server package from cache, `hit` indicates if it was successful
func FromCache(cacheDir, version, dir, platform string) (hit bool, err error) {
	pkg, err := FindPackage(cacheDir, version)
	if err != nil {
		return
	}
	_, filename, method, paths, err := infoForPlatform(pkg, platform)
	if err != nil {
		return
	}

	if !util.Exists(dir) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			err = errors.Wrapf(err, "failed to create dir %s", dir)
			return
		}
	}

	hit, err = download.FromCache(cacheDir, filename, dir, method, paths)
	if !hit || err != nil {
		return
	}

	print.Verb("Using cached package for", version)

	return true, nil
}

// FromNet downloads a server package to the cache, then calls FromCache to finish the job
func FromNet(cacheDir, version, dir, platform string) (err error) {
	print.Info("Downloading package", version, "into", dir)

	pkg, err := FindPackage(cacheDir, version)
	if err != nil {
		return
	}
	location, filename, method, paths, err := infoForPlatform(pkg, platform)
	if err != nil {
		return
	}

	if !util.Exists(dir) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	fullPath, err := download.FromNet(location, cacheDir, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	_, err = method(fullPath, dir, paths)
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", filename)
	}

	ok, err := MatchesChecksum(filepath.Join(dir, getServerBinary(platform)), platform, cacheDir, version)
	if err != nil {
		return errors.Wrap(err, "failed to match checksum")
	} else if !ok {
		return errors.Errorf("server binary does not match checksum for version %s", version)
	}

	return
}

func infoForPlatform(pkg types.RuntimePackage, platform string) (location, filename string, method download.ExtractFunc, paths map[string]string, err error) {
	if platform == "windows" {
		location = pkg.Win32
		method = download.Unzip
		paths = pkg.Win32Paths
	} else if platform == "linux" || platform == "darwin" {
		location = pkg.Linux
		method = download.Untar
		paths = pkg.LinuxPaths
	} else {
		err = errors.Errorf("unsupported OS %s", platform)
		return
	}

	u, err := url.Parse(location)
	if err != nil {
		err = errors.Wrapf(err, "failed to parse location %s", location)
		return
	}
	filename = filepath.Base(u.Path)

	return
}
