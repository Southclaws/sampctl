package runtime

import (
	"context"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

// GetServerPackage checks if a cached package is available and if not, downloads it to dir
func GetServerPackage(version, dir, platform string) (err error) {
	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config dir")
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
	location, _, method, paths, err := infoForPlatform(pkg, platform)
	if err != nil {
		return
	}
	paths = normalizeRuntimePaths(paths, run.DetectRuntimeType(version))

	if !fs.Exists(dir) {
		err = fs.EnsureDir(dir, fs.PermDirPrivate)
		if err != nil {
			err = errors.Wrapf(err, "failed to create dir %s", dir)
			return
		}
	}

	hr, resErr := infraresource.NewHTTPFileResource(location, version, infraresource.ResourceTypeServerBinary)
	if resErr != nil {
		err = resErr
		return
	}
	hr.SetCacheDir(cacheDir)
	hr.SetCacheTTL(0)

	hit, archivePath := hr.Cached(version)
	if !hit {
		hit = false
		return
	}

	files, extractErr := method(archivePath, dir, paths)
	if extractErr != nil {
		hit = false
		err = errors.Wrapf(extractErr, "failed to extract package %s", archivePath)
		return
	}

	if fs.IsPosixPlatform(platform) {
		print.Verb("setting permissions for binaries")
	}
	if err := fs.ChmodAllIfPosix(platform, files, fs.PermFileExec); err != nil {
		return false, err
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
	location, _, method, paths, err := infoForPlatform(pkg, platform)
	if err != nil {
		return
	}
	paths = normalizeRuntimePaths(paths, run.DetectRuntimeType(version))

	if !fs.Exists(dir) {
		err = fs.EnsureDir(dir, fs.PermDirPrivate)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	hr, err := infraresource.NewHTTPFileResource(location, version, infraresource.ResourceTypeServerBinary)
	if err != nil {
		return
	}
	hr.SetCacheDir(cacheDir)
	hr.SetCacheTTL(0)

	if err := hr.Ensure(context.Background(), version, ""); err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	_, fullPath := hr.Cached(version)
	if fullPath == "" {
		return errors.New("failed to locate downloaded server package")
	}

	ok, err := MatchesChecksum(fullPath, platform, cacheDir, version)
	if err != nil {
		innerError := os.Remove(fullPath)
		if innerError != nil {
			return errors.Errorf("failed to remove path for: %s", fullPath)
		}
		return errors.Wrap(err, "failed to match checksum")
	} else if !ok {
		innerError := os.Remove(fullPath)
		if innerError != nil {
			return errors.Errorf("failed to remove path for: %s", fullPath)
		}
		return errors.Errorf("server binary does not match checksum for version %s", version)
	}

	files, err := method(fullPath, dir, paths)
	if err != nil {
		return errors.Wrapf(err, "failed to extract package %s", fullPath)
	}

	if fs.IsPosixPlatform(platform) {
		print.Verb("setting permissions for binaries")
	}
	if err := fs.ChmodAllIfPosix(platform, files, fs.PermFileExec); err != nil {
		return err
	}

	return nil
}

func infoForPlatform(
	pkg download.RuntimePackage,
	platform string,
) (
	location,
	filename string,
	method download.ExtractFunc,
	paths map[string]string,
	err error,
) {
	switch platform {
	case "windows":
		location = pkg.Win32
		method = download.Unzip
		paths = pkg.Win32Paths
	case "linux", "darwin":
		location = pkg.Linux
		method = download.Untar
		paths = pkg.LinuxPaths
	default:
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

func normalizeRuntimePaths(paths map[string]string, runtimeType run.RuntimeType) map[string]string {
	if runtimeType != run.RuntimeTypeOpenMP {
		return paths
	}

	out := make(map[string]string, len(paths))
	for src, dst := range paths {
		base := filepath.Base(src)
		switch {
		case base == "omp-server" && dst == "samp03svr":
			dst = "omp-server"
		case base == "omp-server.exe" && dst == "samp-server.exe":
			dst = "omp-server.exe"
		}
		out[src] = dst
	}
	return out
}
