//nolint:dupl,golint
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
	runtimesFile := cache.Path(cacheDir, "runtimes.json")
	runtimes, refreshed, err := cache.GetOrRefreshJSON[Runtimes](
		context.Background(),
		runtimesFile,
		time.Hour*24*7,
		0o755,
		0o644,
		func(ctx context.Context) (Runtimes, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/sampctl/runtimes/master/runtimes.json", nil)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to create request")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to download package list")
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return Runtimes{}, errors.Errorf("package list status %s", resp.Status)
			}
			var out Runtimes
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to decode package list")
			}
			return out, nil
		},
	)
	if err != nil {
		return Runtimes{}, err
	}
	if refreshed {
		fmt.Fprintln(os.Stderr, "updating runtimes list...") // nolint:gas
	}
	return runtimes, nil
}

// UpdateRuntimeList downloads a list of all runtime packages to a file in the cache directory
func UpdateRuntimeList(cacheDir string) (err error) {
	runtimesFile := cache.Path(cacheDir, "runtimes.json")
	_, _, err = cache.GetOrRefreshJSON[Runtimes](
		context.Background(),
		runtimesFile,
		-1,
		0o755,
		0o644,
		func(ctx context.Context) (Runtimes, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/sampctl/runtimes/master/runtimes.json", nil)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to create request")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to download package list")
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return Runtimes{}, errors.Errorf("package list status %s", resp.Status)
			}
			var out Runtimes
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return Runtimes{}, errors.Wrap(err, "failed to decode package list")
			}
			return out, nil
		},
	)
	return err
}

func WriteRuntimeCacheFile(cacheDir string, data []byte) error {
	if err := cache.WriteFileAtomic(cache.Path(cacheDir, "runtimes.json"), data, 0o755, 0o644); err != nil {
		return errors.Wrap(err, "failed to write runtime list to file")
	}
	return nil
}
