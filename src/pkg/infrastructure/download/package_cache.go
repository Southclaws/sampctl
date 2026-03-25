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
	return GetPackageListContext(context.Background(), cacheDir)
}

func GetPackageListContext(ctx context.Context, cacheDir string) (packages []pawnpackage.Package, err error) {
	return GetPackageListWithClientContext(ctx, cacheDir, http.DefaultClient)
}

func GetPackageListWithClient(cacheDir string, client HTTPDoer) (packages []pawnpackage.Package, err error) {
	return GetPackageListWithClientContext(context.Background(), cacheDir, client)
}

func GetPackageListWithClientContext(ctx context.Context, cacheDir string, client HTTPDoer) (packages []pawnpackage.Package, err error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}
	packageFile := fs.Join(cacheDir, "packages.json")
	packages, refreshed, err := cache.GetOrRefreshJSON(cache.JSONCacheRequest[[]pawnpackage.Package]{
		Context:  ctx,
		Path:     packageFile,
		TTL:      time.Hour * 24 * 7,
		DirPerm:  fs.PermDirPrivate,
		FilePerm: fs.PermFileShared,
		Fetch: func(ctx context.Context) ([]pawnpackage.Package, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://list.packages.sampctl.com", nil)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create request")
			}
			resp, err := client.Do(req)
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
	})
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
	return UpdatePackageListContext(context.Background(), cacheDir)
}

func UpdatePackageListContext(ctx context.Context, cacheDir string) (err error) {
	return UpdatePackageListWithClientContext(ctx, cacheDir, http.DefaultClient)
}

func UpdatePackageListWithClient(cacheDir string, client HTTPDoer) (err error) {
	return UpdatePackageListWithClientContext(context.Background(), cacheDir, client)
}

func UpdatePackageListWithClientContext(ctx context.Context, cacheDir string, client HTTPDoer) (err error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}
	packageFile := fs.Join(cacheDir, "packages.json")
	_, _, err = cache.GetOrRefreshJSON(cache.JSONCacheRequest[[]pawnpackage.Package]{
		Context:  ctx,
		Path:     packageFile,
		TTL:      -1,
		DirPerm:  fs.PermDirPrivate,
		FilePerm: fs.PermFileShared,
		Fetch: func(ctx context.Context) ([]pawnpackage.Package, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://list.packages.sampctl.com", nil)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create request")
			}
			resp, err := client.Do(req)
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
	})
	return err
}

func WritePackageCacheFile(cacheDir string, data []byte) error {
	if err := fs.WriteFileAtomic(fs.Join(cacheDir, "packages.json"), data, fs.PermDirPrivate, fs.PermFileShared); err != nil {
		return errors.Wrap(err, "failed to write package list to file")
	}
	return nil
}
