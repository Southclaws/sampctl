// Package download handles downloading and extracting sa-mp server versions.
// Packages are cached in ~/.samp to avoid unnecessary downloads.
package download

import (
	"context"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

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

	client := &http.Client{Timeout: 60 * time.Second}

	const maxAttempts = 5
	backoff := func(attempt int) time.Duration {
		base := 250 * time.Millisecond
		d := base << max(0, attempt-1)
		if d > 4*time.Second {
			d = 4 * time.Second
		}
		j := time.Duration(rand.Intn(200)) * time.Millisecond
		return d + j
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
		if reqErr != nil {
			return "", errors.Wrap(reqErr, "failed to create request")
		}
		req.Header.Set("User-Agent", "sampctl")
		req.Header.Set("Accept", "application/octet-stream, application/*, */*")

		resp, doErr := client.Do(req)
		if doErr != nil {
			lastErr = errors.Wrapf(doErr, "failed to download package from %s", location)
			if attempt < maxAttempts {
				time.Sleep(backoff(attempt))
				continue
			}
			return "", lastErr
		}

		func() {
			defer func() {
				_ = resp.Body.Close()
			}()

			if resp.StatusCode != http.StatusOK {
				if (resp.StatusCode >= 500 && resp.StatusCode <= 599) || resp.StatusCode == http.StatusTooManyRequests {
					_, _ = io.CopyN(io.Discard, resp.Body, 32*1024)
					lastErr = errors.Errorf("unexpected status code given %d", resp.StatusCode)
					if attempt < maxAttempts {
						if ra := resp.Header.Get("Retry-After"); ra != "" {
							if secs, convErr := strconv.Atoi(strings.TrimSpace(ra)); convErr == nil && secs > 0 && secs <= 10 {
								time.Sleep(time.Duration(secs) * time.Second)
								return
							}
						}
						time.Sleep(backoff(attempt))
						return
					}
					err = lastErr
					return
				}

				lastErr = errors.Errorf("unexpected status code given %d", resp.StatusCode)
				err = lastErr
				return
			}

			ct := resp.Header.Get("Content-Type")
			if ct != "" {
				if t, _, parseErr := mime.ParseMediaType(ct); parseErr == nil {
					if !(strings.HasPrefix(t, "application/") || strings.HasPrefix(t, "text/")) {
						lastErr = errors.Errorf("content has unexpected content type %s", t)
						err = lastErr
						return
					}
				}
			}

			if writeErr := fs.WriteFromReaderAtomic(cachePath, resp.Body, fs.PermDirPrivate, fs.PermFileShared); writeErr != nil {
				lastErr = errors.Wrap(writeErr, "failed to write package to cache")
				err = lastErr
				return
			}
			err = nil
		}()

		if err == nil {
			return cachePath, nil
		}
		if attempt < maxAttempts {
			time.Sleep(backoff(attempt))
			continue
		}
		return "", err
	}

	if lastErr == nil {
		lastErr = errors.New("download failed")
	}
	return "", lastErr
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
