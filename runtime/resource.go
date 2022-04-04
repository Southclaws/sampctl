package runtime

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
)

func getServerBinary(cacheDir, version, platform string) (binary string) {
	pkg, err := FindPackage(cacheDir, version)
	if err != nil {
		return
	}

	var (
		paths map[string]string
		part  string
	)
	switch platform {
	case "windows":
		paths = pkg.Win32Paths
		part = "samp-server"
	case "linux", "darwin":
		paths = pkg.LinuxPaths
		part = "samp03svr"
	default:
		return
	}

	for _, destination := range paths {
		if strings.Contains(destination, part) {
			binary = destination
			break
		}
	}

	return
}

func getNpcBinary(cacheDir, version, platform string) (binary string) {
	pkg, err := FindPackage(cacheDir, version)
	if err != nil {
		return
	}

	var (
		paths map[string]string
		part  string
	)
	switch platform {
	case "windows":
		paths = pkg.Win32Paths
		part = "npc"
	case "linux", "darwin":
		paths = pkg.LinuxPaths
		part = "npc"
	default:
		return
	}

	for _, destination := range paths {
		if strings.Contains(destination, part) {
			binary = destination
			break
		}
	}

	return
}

func getAnnounceBinary(cacheDir, version, platform string) (binary string) {
	pkg, err := FindPackage(cacheDir, version)
	if err != nil {
		return
	}

	var (
		paths map[string]string
		part  string
	)
	switch platform {
	case "windows":
		paths = pkg.Win32Paths
		part = "announ"
	case "linux", "darwin":
		paths = pkg.LinuxPaths
		part = "announ"
	default:
		return
	}

	for _, destination := range paths {
		if strings.Contains(destination, part) {
			binary = destination
			break
		}
	}

	return
}

// MatchesChecksum checks if the file at the given path src is the correct file for the specified
// runtime package via MD5 sum
func MatchesChecksum(src, platform, cacheDir, version string) (ok bool, err error) {
	print.Verb("attempting to match checksum from source", src, "on platform", platform, "with the cache located at", cacheDir, "and with version", version)

	pkg, err := FindPackage(cacheDir, version)
	if err != nil {
		return
	}

	contents, err := ioutil.ReadFile(src)
	if err != nil {
		return false, errors.Wrap(err, "failed to read downloaded server package")
	}

	print.Verb("checksum for linux/mac", pkg.LinuxChecksum, "and for windows", pkg.Win32Checksum)

	want := ""
	switch platform {
	case "windows":
		want = pkg.Win32Checksum
	case "linux", "darwin":
		want = pkg.LinuxChecksum
	default:
		return false, errors.New("platform not supported")
	}

	checksum := md5.Sum([]byte(contents))
	fmt.Printf("has: %s, wants: %s ", checksum, want)

	return hex.EncodeToString(checksum[:]) == want, nil
}

// FindPackage returns a server resource package for the given version or nil if it's invalid
func FindPackage(cacheDir, version string) (runtime download.RuntimePackage, err error) {
	return findPackageRecursive(cacheDir, version, true)
}

func findPackageRecursive(cacheDir, version string, aliases bool) (runtime download.RuntimePackage, err error) {
	packages, err := download.GetRuntimeList(cacheDir)
	if err != nil {
		return
	}

	for _, runtime = range packages.Packages {
		if runtime.Version == version {
			return
		}
	}
	if aliases {
		for alias, target := range packages.Aliases {
			if alias == version {
				return findPackageRecursive(cacheDir, target, false)
			}
		}
	}

	err = errors.Errorf("server package for '%s' not found", version)
	return
}
