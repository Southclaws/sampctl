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
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

// GetPackageList gets a list of known packages from the sampctl package service, if the list does
// not exist locally, it is downloaded and cached for future use.
func GetPackageList(cacheDir string) (packages []pawnpackage.Package, err error) {
	packageFile := fs.Join(cacheDir, "packages.json")
	packages, refreshed, err := cache.GetOrRefreshJSON[[]pawnpackage.Package](
		context.Background(),
		packageFile,
		time.Hour*24*7,
		fs.PermDirPrivate,
		fs.PermFileShared,
		func(ctx context.Context) ([]pawnpackage.Package, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://list.packages.sampctl.com", nil)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create request")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, errors.Wrap(err, "failed to download package list")
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return nil, errors.Errorf("package list status %s", resp.Status)
			}
			var out []pawnpackage.Package
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return nil, errors.Wrap(err, "failed to decode package list")
			}
			return out, nil
		},
	)
	if err != nil {
		return nil, err
	}
	if refreshed {
		fmt.Fprintln(os.Stderr, "updating package list...") //nolint
	}
	return packages, nil
}

// UpdatePackageList downloads a list of all packages to a file in the cache directory
func UpdatePackageList(cacheDir string) (err error) {
	packageFile := fs.Join(cacheDir, "packages.json")
	_, _, err = cache.GetOrRefreshJSON[[]pawnpackage.Package](
		context.Background(),
		packageFile,
		-1,
		fs.PermDirPrivate,
		fs.PermFileShared,
		func(ctx context.Context) ([]pawnpackage.Package, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://list.packages.sampctl.com", nil)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create request")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, errors.Wrap(err, "failed to download package list")
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return nil, errors.Errorf("package list status %s", resp.Status)
			}
			var out []pawnpackage.Package
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return nil, errors.Wrap(err, "failed to decode package list")
			}
			return out, nil
		},
	)
	return err
}

func WritePackageCacheFile(cacheDir string, data []byte) error {
	if err := fs.WriteFileAtomic(fs.Join(cacheDir, "packages.json"), data, fs.PermDirPrivate, fs.PermFileShared); err != nil {
		return errors.Wrap(err, "failed to write package list to file")
	}
	return nil
}
