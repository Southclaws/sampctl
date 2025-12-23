// Package download handles downloading and extracting sa-mp server versions.
// Packages are cached in ~/.samp to avoid unnecessary downloads.
package download

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"github.com/mitchellh/go-homedir"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// ExtractFunc represents a function responsible for extracting a set of files from an archive to
// a directory. The map argument contains a map of source files in the archive to target file
// locations on the host filesystem (absolute paths).
type ExtractFunc func(string, string, map[string]string) (map[string]string, error)

const (
	// ExtractZip is an extract function for .zip packages
	ExtractZip = "zip"
	// ExtractTgz is an extract function for .tar.gz packages
	ExtractTgz = "tgz"
)

// ExtractFuncFromName returns an extract function for a given name
func ExtractFuncFromName(name string) ExtractFunc {
	switch name {
	case ExtractZip:
		return Unzip
	case ExtractTgz:
		return Untar
	default:
		return nil
	}
}

// MigrateOldConfig migrates old config to the new path.
func MigrateOldConfig(cacheDir string) error {
	home, err := homedir.Dir()
	if err != nil {
		return errors.Wrap(err, "failed to get home directory")
	}

	// Attempt to check if the old cache directory exists and if it does
	// move it to the new cache directory
	oldCacheDir := filepath.Join(home, ".samp")
	if _, err := os.Stat(oldCacheDir); !os.IsNotExist(err) {
		err = copy.Copy(oldCacheDir, cacheDir)
		if err != nil {
			return errors.Wrap(err, "Failed to copy old cache directory to new cache directory")
		}

		err = os.RemoveAll(oldCacheDir)
		if err != nil {
			return errors.Wrap(err, "Failed to remove all the contents of the old cache directory")
		}
	}

	return nil
}

// FromCache first checks if a file is cached, then
func FromCache(cacheDir, filename, dir string, method ExtractFunc, paths map[string]string, platform string) (hit bool, err error) {
	path := filepath.Join(cacheDir, filename)

	if !fs.Exists(path) {
		hit = false
		return
	}

	var files map[string]string
	files, err = method(path, dir, paths)
	if err != nil {
		hit = false
		err = errors.Wrapf(err, "failed to unzip package %s", path)
		return
	}

	if platform == "linux" || platform == "darwin" {
		print.Verb("setting permissions for binaries")
		for _, file := range files {
			err = os.Chmod(file, 0o700)
			if err != nil {
				return
			}
		}
	}

	return true, nil
}

func FromNet(ctx context.Context, location, cachePath string) (result string, err error) {
	print.Verb("attempting to download package from", location, "to", cachePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to create request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to download package from %s", location)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			panic(errClose)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("unexpected status code given %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		if t, _, err := mime.ParseMediaType(ct); err == nil {
			if !(strings.HasPrefix(t, "application/") || strings.HasPrefix(t, "text/")) {
				return "", errors.Errorf("content has unexpected content type %s", t)
			}
		}
	}

	if err := fs.WriteFromReaderAtomic(cachePath, resp.Body, fs.PermDirPrivate, fs.PermFileShared); err != nil {
		return "", errors.Wrap(err, "failed to write package to cache")
	}

	return cachePath, nil
}

// ReleaseAssetByPattern downloads a resource file, which is a GitHub release asset
func ReleaseAssetByPattern(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	matcher *regexp.Regexp,
	dir,
	outputFile,
	cacheDir string,
) (filename, tag string, err error) {
	var (
		asset  *github.ReleaseAsset
		assets = make([]string, 1)
	)

	var release *github.RepositoryRelease
	if meta.Tag == "" {
		release, err = getLatestReleaseOrPreRelease(ctx, gh, meta.User, meta.Repo)
	} else {
		release, _, err = gh.Repositories.GetReleaseByTag(ctx, meta.User, meta.Repo, meta.Tag)
	}
	if err != nil {
		return
	}

	for _, a := range release.Assets {
		rel := a
		if matcher.MatchString(*a.Name) {
			asset = &rel
			break
		}
		assets = append(assets, *a.Name)
	}
	if asset == nil {
		err = errors.Errorf("resource matcher '%s' does not match any release assets from '%v'", matcher, assets)
		return
	}
	tag = release.GetTagName()
	if dir == "" {
		dir = filepath.Join("assets", meta.User, meta.Repo, tag)
	}

	baseDir := dir
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(cacheDir, baseDir)
	}

	if outputFile == "" {
		var u *url.URL
		u, err = url.Parse(*asset.BrowserDownloadURL)
		if err != nil {
			err = errors.Wrap(err, "failed to parse download URL from GitHub API")
			return
		}
		outputFile = filepath.Base(u.Path)
	} else {
		outputFile = filepath.Base(outputFile)
	}

	destination := filepath.Join(baseDir, outputFile)
	filename, err = FromNet(ctx, *asset.BrowserDownloadURL, destination)
	if err != nil {
		return
	}

	return filename, tag, nil
}

func getLatestReleaseOrPreRelease(
	ctx context.Context,
	gh *github.Client,
	owner string,
	repo string,
) (release *github.RepositoryRelease, err error) {
	releases, _, err := gh.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{})
	if err != nil {
		err = errors.Wrap(err, "failed to list releases")
		return
	}

	if len(releases) == 0 {
		err = errors.New("no releases available")
		return
	}

	release = releases[0]

	return
}
