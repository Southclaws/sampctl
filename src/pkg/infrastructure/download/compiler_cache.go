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

// Compilers is a list of compilers for each platform
type Compilers map[string]Compiler

// Compiler represents a compiler package for a specific OS
type Compiler struct {
	Match  string            `json:"match"`  // the release asset name pattern
	Method string            `json:"method"` // the extraction method
	Binary string            `json:"binary"` // execution binary
	Paths  map[string]string `json:"paths"`  // map of files to their target locations
}

// GetCompilerList gets a list of known compiler packages from the sampctl repo, if the list does not
// exist locally, it is downloaded and cached for future use.
func GetCompilerList(cacheDir string) (compilers Compilers, err error) {
	return GetCompilerListContext(context.Background(), cacheDir)
}

func GetCompilerListContext(ctx context.Context, cacheDir string) (compilers Compilers, err error) {
	return GetCompilerListWithClientContext(ctx, cacheDir, http.DefaultClient)
}

func GetCompilerListWithClientContext(ctx context.Context, cacheDir string, client HTTPDoer) (compilers Compilers, err error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}

	compilersFile := fs.Join(cacheDir, "compilers.json")
	var refreshErr error

	if !cache.IsFresh(compilersFile, time.Hour*24*7) {
		fmt.Fprintln(os.Stderr, "updating compiler list...") //nolint:gosec
		refreshErr = UpdateCompilerListWithClientContext(ctx, cacheDir, client)
		if refreshErr != nil {
			fmt.Fprintln(os.Stderr, errors.Wrap(refreshErr, "failed to update compiler list"))
		}
	}

	compilers, err = cache.ReadJSON[Compilers](compilersFile)
	if err != nil {
		if refreshErr != nil {
			return nil, errors.Wrap(refreshErr, "failed to refresh compiler cache")
		}
		return nil, errors.Wrap(err, "failed to read package cache file")
	}
	return compilers, nil
}

// UpdateCompilerList downloads a list of all runtime packages to a file in the cache directory
func UpdateCompilerList(cacheDir string) (err error) {
	return UpdateCompilerListContext(context.Background(), cacheDir)
}

func UpdateCompilerListContext(ctx context.Context, cacheDir string) (err error) {
	return UpdateCompilerListWithClientContext(ctx, cacheDir, http.DefaultClient)
}

func UpdateCompilerListWithClientContext(ctx context.Context, cacheDir string, client HTTPDoer) (err error) {
	if client == nil {
		client = http.DefaultClient
	}
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/sampctl/compilers/master/compilers.json", nil)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to download package list")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.Errorf("package list status %s", resp.Status)
	}

	var compilers Compilers
	err = json.NewDecoder(resp.Body).Decode(&compilers)
	if err != nil {
		return errors.Wrap(err, "failed to decode package list")
	}

	if err := fs.WriteJSONAtomic(fs.Join(cacheDir, "compilers.json"), compilers, fs.PermDirPrivate, fs.PermFileShared); err != nil {
		return errors.Wrap(err, "failed to write compilers list to file")
	}
	return nil
}

func WriteCompilerCacheFile(cacheDir string, data []byte) error {
	if err := fs.WriteFileAtomic(fs.Join(cacheDir, "compilers.json"), data, fs.PermDirPrivate, fs.PermFileShared); err != nil {
		return errors.Wrap(err, "failed to write compilers list to file")
	}
	return nil
}
