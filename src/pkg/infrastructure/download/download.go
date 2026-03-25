// Package download handles downloading and extracting sa-mp server versions.
// Packages are cached in ~/.samp to avoid unnecessary downloads.
package download

import (
	"context"
	"io"
	"math/rand/v2"
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

var (
	fromNetMaxAttempts   = 5
	fromNetSleep         = time.Sleep
	fromNetClientFactory = func() *http.Client { return &http.Client{Timeout: 60 * time.Second} }
	fromNetBackoff       = func(attempt int) time.Duration {
		base := 250 * time.Millisecond
		d := base << max(0, attempt-1)
		if d > 4*time.Second {
			d = 4 * time.Second
		}
		j := time.Duration(rand.IntN(200)) * time.Millisecond
		return d + j
	}
)

// ExtractFunc represents a function responsible for extracting a set of files from an archive to
// a directory. The map argument contains a map of source files in the archive to target file
// locations on the host filesystem (absolute paths).
type ExtractFunc func(string, string, map[string]string) (map[string]string, error)

// CacheExtractRequest describes a cached archive extraction operation.
type CacheExtractRequest struct {
	CacheDir string
	Filename string
	Dir      string
	Method   ExtractFunc
	Paths    map[string]string
	Platform string
}

// ReleaseAssetRequest describes a GitHub release asset download using a github.Client.
type ReleaseAssetRequest struct {
	Context    context.Context
	Client     *github.Client
	Meta       versioning.DependencyMeta
	Matcher    *regexp.Regexp
	Dir        string
	OutputFile string
	CacheDir   string
}

// ReleaseAssetAPIRequest describes a GitHub release asset download using the narrow releases API.
type ReleaseAssetAPIRequest struct {
	Context    context.Context
	Client     GitHubReleasesAPI
	Meta       versioning.DependencyMeta
	Matcher    *regexp.Regexp
	Dir        string
	OutputFile string
	CacheDir   string
}

// ReleaseAssetDownloadRequest describes the final transfer of a selected release asset.
type ReleaseAssetDownloadRequest struct {
	Context     context.Context
	Client      GitHubReleasesAPI
	Meta        versioning.DependencyMeta
	Asset       *github.ReleaseAsset
	Destination string
}

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

// FromCache first checks if a file is cached, then extracts it if present.
func FromCache(request CacheExtractRequest) (hit bool, err error) {
	path := filepath.Join(request.CacheDir, request.Filename)

	if !fs.Exists(path) {
		hit = false
		return
	}

	var files map[string]string
	files, err = request.Method(path, request.Dir, request.Paths)
	if err != nil {
		hit = false
		err = errors.Wrapf(err, "failed to unzip package %s", path)
		return
	}

	if fs.IsPosixPlatform(request.Platform) {
		print.Verb("setting permissions for binaries")
	}
	if err := fs.ChmodAllIfPosix(request.Platform, files, fs.PermFileExec); err != nil {
		hit = false
		return false, err
	}

	return true, nil
}

func FromNet(ctx context.Context, location, cachePath string) (result string, err error) {
	return FromNetWithClient(ctx, fromNetClientFactory(), location, cachePath)
}

func FromNetWithClient(ctx context.Context, client HTTPDoer, location, cachePath string) (result string, err error) {
	print.Verb("attempting to download package from", location, "to", cachePath)
	if client == nil {
		client = fromNetClientFactory()
	}
	maxAttempts := fromNetMaxAttempts
	backoff := fromNetBackoff

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
				fromNetSleep(backoff(attempt))
				continue
			}
			return "", lastErr
		}

		if resp.StatusCode != http.StatusOK {
			retryable := (resp.StatusCode >= 500 && resp.StatusCode <= 599) || resp.StatusCode == http.StatusTooManyRequests
			_, _ = io.CopyN(io.Discard, resp.Body, 32*1024)
			_ = resp.Body.Close()
			lastErr = errors.Errorf("unexpected status code given %d", resp.StatusCode)
			err = lastErr
			if !retryable {
				// Non-retryable status codes (e.g., 401/403/404) won't improve with retries.
				return "", err
			}
			if attempt < maxAttempts {
				if resp.StatusCode == http.StatusTooManyRequests {
					if ra := resp.Header.Get("Retry-After"); ra != "" {
						if secs, convErr := strconv.Atoi(strings.TrimSpace(ra)); convErr == nil && secs > 0 && secs <= 10 {
							fromNetSleep(time.Duration(secs) * time.Second)
							continue
						}
					}
				}
				fromNetSleep(backoff(attempt))
				continue
			}
			return "", err
		}

		ct := resp.Header.Get("Content-Type")
		if ct != "" {
			if t, _, parseErr := mime.ParseMediaType(ct); parseErr == nil {
				if !(strings.HasPrefix(t, "application/") || strings.HasPrefix(t, "text/")) {
					_ = resp.Body.Close()
					lastErr = errors.Errorf("content has unexpected content type %s", t)
					err = lastErr
					if attempt < maxAttempts {
						fromNetSleep(backoff(attempt))
						continue
					}
					return "", err
				}
			}
		}

		if writeErr := fs.WriteFromReaderAtomic(cachePath, resp.Body, fs.PermDirPrivate, fs.PermFileShared); writeErr != nil {
			_ = resp.Body.Close()
			lastErr = errors.Wrap(writeErr, "failed to write package to cache")
			err = lastErr
			if attempt < maxAttempts {
				fromNetSleep(backoff(attempt))
				continue
			}
			return "", err
		}
		_ = resp.Body.Close()
		return cachePath, nil
	}

	if lastErr == nil {
		lastErr = errors.New("download failed")
	}
	return "", lastErr
}

// ReleaseAssetByPattern downloads a resource file, which is a GitHub release asset
func ReleaseAssetByPattern(request ReleaseAssetRequest) (filename, tag string, err error) {
	return ReleaseAssetByPatternWithAPI(ReleaseAssetAPIRequest{
		Context:    request.Context,
		Client:     githubClientReleasesAdapter{client: request.Client},
		Meta:       request.Meta,
		Matcher:    request.Matcher,
		Dir:        request.Dir,
		OutputFile: request.OutputFile,
		CacheDir:   request.CacheDir,
	})
}

func ReleaseAssetByPatternWithAPI(request ReleaseAssetAPIRequest) (filename, tag string, err error) {
	var (
		asset  *github.ReleaseAsset
		assets = make([]string, 1)
	)

	var release *github.RepositoryRelease
	if request.Meta.Tag == "" {
		release, err = getLatestReleaseOrPreRelease(request.Context, request.Client, request.Meta.User, request.Meta.Repo)
	} else {
		release, _, err = request.Client.GetReleaseByTag(request.Context, request.Meta.User, request.Meta.Repo, request.Meta.Tag)
	}
	if err != nil {
		return
	}

	for _, a := range release.Assets {
		rel := a
		if request.Matcher.MatchString(*a.Name) {
			asset = &rel
			break
		}
		assets = append(assets, *a.Name)
	}
	if asset == nil {
		err = errors.Errorf("resource matcher '%s' does not match any release assets from '%v'", request.Matcher, assets)
		return
	}
	tag = release.GetTagName()
	if request.Dir == "" {
		request.Dir = filepath.Join("assets", request.Meta.User, request.Meta.Repo, tag)
	}

	baseDir := request.Dir
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(request.CacheDir, baseDir)
	}

	if request.OutputFile == "" {
		if name := asset.GetName(); name != "" {
			request.OutputFile = name
		} else if asset.BrowserDownloadURL != nil && *asset.BrowserDownloadURL != "" {
			var u *url.URL
			u, err = url.Parse(*asset.BrowserDownloadURL)
			if err != nil {
				err = errors.Wrap(err, "failed to parse download URL from GitHub API")
				return
			}
			request.OutputFile = filepath.Base(u.Path)
		} else {
			err = errors.New("release asset has no name or download URL")
			return
		}
	} else {
		request.OutputFile = filepath.Base(request.OutputFile)
	}

	destination := filepath.Join(baseDir, request.OutputFile)
	filename, err = downloadReleaseAsset(ReleaseAssetDownloadRequest{
		Context:     request.Context,
		Client:      request.Client,
		Meta:        request.Meta,
		Asset:       asset,
		Destination: destination,
	})
	if err != nil {
		return
	}

	return filename, tag, nil
}

func downloadReleaseAsset(request ReleaseAssetDownloadRequest) (string, error) {
	if request.Asset == nil {
		return "", errors.New("no release asset selected")
	}

	if request.Client != nil && request.Asset.GetID() != 0 {
		rc, redirectURL, err := request.Client.DownloadReleaseAsset(request.Context, request.Meta.User, request.Meta.Repo, request.Asset.GetID())
		if err != nil {
			return "", errors.Wrap(err, "failed to download release asset via GitHub API")
		}
		if redirectURL != "" {
			return FromNet(request.Context, redirectURL, request.Destination)
		}
		if rc == nil {
			return "", errors.New("empty response body for release asset download")
		}
		defer rc.Close()
		if writeErr := fs.WriteFromReaderAtomic(request.Destination, rc, fs.PermDirPrivate, fs.PermFileShared); writeErr != nil {
			return "", errors.Wrap(writeErr, "failed to write package to cache")
		}
		return request.Destination, nil
	}

	if request.Asset.BrowserDownloadURL == nil || *request.Asset.BrowserDownloadURL == "" {
		return "", errors.New("release asset has no download URL")
	}
	return FromNet(request.Context, *request.Asset.BrowserDownloadURL, request.Destination)
}

func getLatestReleaseOrPreRelease(
	ctx context.Context,
	gh GitHubReleasesAPI,
	owner string,
	repo string,
) (release *github.RepositoryRelease, err error) {
	releases, _, err := gh.ListReleases(ctx, owner, repo, &github.ListOptions{})
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
