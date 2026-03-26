//nolint:dupl
package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/cache"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

// Runtimes is a collection of Package objects for sorting
type Runtimes struct {
	Aliases  map[string]string `json:"aliases"`
	Packages []RuntimePackage  `json:"packages"`
}

// RuntimePackage represents a SA:MP server version, it stores both platform filenames and a checksum
type RuntimePackage struct {
	Version       string            `json:"version"`
	Linux         string            `json:"linux"`
	Win32         string            `json:"win32"`
	LinuxChecksum string            `json:"linux_checksum"`
	Win32Checksum string            `json:"win32_checksum"`
	LinuxPaths    map[string]string `json:"linux_paths"`
	Win32Paths    map[string]string `json:"win32_paths"`
}

// GetRuntimeList gets a list of known runtime packages from the sampctl repo, if the list does not
// exist locally, it is downloaded and cached for future use.
func GetRuntimeList(cacheDir string) (runtimes Runtimes, err error) {
	return GetRuntimeListContext(context.Background(), cacheDir)
}

func GetRuntimeListContext(ctx context.Context, cacheDir string) (runtimes Runtimes, err error) {
	return GetRuntimeListWithClientContext(ctx, cacheDir, http.DefaultClient)
}

func GetRuntimeListWithClientContext(ctx context.Context, cacheDir string, client HTTPDoer) (runtimes Runtimes, err error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runtimesFile := fs.Join(cacheDir, "runtimes.json")
	runtimes, refreshed, err := cache.GetOrRefreshJSON(cache.JSONCacheRequest[Runtimes]{
		Context:  ctx,
		Path:     runtimesFile,
		TTL:      time.Hour * 24 * 7,
		DirPerm:  fs.PermDirPrivate,
		FilePerm: fs.PermFileShared,
		Fetch: func(ctx context.Context) (out Runtimes, err error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/sampctl/runtimes/master/runtimes.json", nil)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to create request")
			}
			resp, err := client.Do(req)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to download package list")
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
					err = errors.Wrap(closeErr, "failed to close runtime list response body")
				}
			}()
			if resp.StatusCode != 200 {
				return Runtimes{}, errors.Errorf("package list status %s", resp.Status)
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to decode package list")
			}
			return out, nil
		},
	})
	if err != nil {
		return Runtimes{}, err
	}
	if refreshed {
		fmt.Fprintln(os.Stderr, "updating runtimes list...") //nolint:gosec
	}
	return runtimes, nil
}

// UpdateRuntimeList downloads a list of all runtime packages to a file in the cache directory
func UpdateRuntimeList(cacheDir string) (err error) {
	return UpdateRuntimeListContext(context.Background(), cacheDir)
}

func UpdateRuntimeListContext(ctx context.Context, cacheDir string) (err error) {
	return UpdateRuntimeListWithClientContext(ctx, cacheDir, http.DefaultClient)
}

func UpdateRuntimeListWithClientContext(ctx context.Context, cacheDir string, client HTTPDoer) (err error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runtimesFile := fs.Join(cacheDir, "runtimes.json")
	_, _, err = cache.GetOrRefreshJSON(cache.JSONCacheRequest[Runtimes]{
		Context:  ctx,
		Path:     runtimesFile,
		TTL:      -1,
		DirPerm:  fs.PermDirPrivate,
		FilePerm: fs.PermFileShared,
		Fetch: func(ctx context.Context) (out Runtimes, err error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/sampctl/runtimes/master/runtimes.json", nil)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to create request")
			}
			resp, err := client.Do(req)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to download package list")
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
					err = errors.Wrap(closeErr, "failed to close runtime list response body")
				}
			}()
			if resp.StatusCode != 200 {
				return Runtimes{}, errors.Errorf("package list status %s", resp.Status)
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to decode package list")
			}
			return out, nil
		},
	})
	return err
}

func WriteRuntimeCacheFile(cacheDir string, data []byte) error {
	if err := fs.WriteFileAtomic(fs.Join(cacheDir, "runtimes.json"), data, fs.PermDirPrivate, fs.PermFileShared); err != nil {
		return errors.Wrap(err, "failed to write runtime list to file")
	}
	return nil
}
