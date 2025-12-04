package pawnpackage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// PackageFromDir attempts to parse a pawn.json or pawn.yaml file from a directory
func PackageFromDir(dir string) (pkg Package, err error) {
	jsonPath := filepath.Join(dir, "pawn.json")
	yamlPath := filepath.Join(dir, "pawn.yaml")
	jsonExists := util.Exists(jsonPath)
	yamlExists := util.Exists(yamlPath)

	switch {
	case jsonExists && yamlExists:
		return pkg, errors.New("found both pawn.json and pawn.yaml; please keep only one package definition file")
	case jsonExists:
		return readPackageDefinition(jsonPath, "json")
	case yamlExists:
		return readPackageDefinition(yamlPath, "yaml")
	default:
		print.Verb("no package definition file (pawn.{json|yaml})")
		return pkg, nil
	}
}

func readPackageDefinition(path, format string) (pkg Package, err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return pkg, errors.Wrapf(err, "failed to read configuration from '%s'", path)
	}

	if format == "json" {
		err = json.Unmarshal(data, &pkg)
	} else {
		err = yaml.Unmarshal(data, &pkg)
	}
	if err != nil {
		return pkg, errors.Wrapf(err, "failed to parse configuration from '%s'", path)
	}

	pkg.Format = format

	return pkg, nil
}

// GetCachedPackage returns a package using the cached copy, if it exists
func GetCachedPackage(meta versioning.DependencyMeta, cacheDir string) (pkg Package, err error) {
	path := meta.CachePath(cacheDir)
	return PackageFromDir(path)
}

// GetRemotePackage attempts to get a package definition for the given dependency meta.
// It first checks the the sampctl central repository, if that fails it falls back to using the
// repository for the package itself. This means upstream changes to plugins can be first staged in
// the official central repository before being pulled to the package specific repository.
func GetRemotePackage(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	pkg, err = PackageFromOfficialRepo(ctx, client, meta)
	if err != nil {
		return PackageFromRepo(ctx, client, meta)
	}
	return
}

// PackageFromRepo attempts to get a package from the given package definition's public repo
func PackageFromRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	repo, _, err := client.Repositories.Get(ctx, meta.User, meta.Repo)
	if err != nil {
		return
	}
	var resp *http.Response

	resp, err = http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/pawn.json",
		meta.User, meta.Repo, *repo.DefaultBranch,
	))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return packageFromJSONResponse(resp, meta)
	}

	resp, err = http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/pawn.yaml",
		meta.User, meta.Repo, *repo.DefaultBranch,
	))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return packageFromYAMLResponse(resp, meta)
	}

	return pkg, errors.Wrap(err, "package does not point to a valid remote package")
}

// PackageFromOfficialRepo attempts to get a package from the sampctl/plugins official repository
// this repo is mainly only used for testing plugins before being PR'd into their respective repos.
func PackageFromOfficialRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	resp, err := http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json",
		meta.User, meta.Repo,
	))
	if err != nil {
		err = errors.Wrapf(err, "failed to get plugin '%s' from official repo", meta)
		return
	}
	defer resp.Body.Close()
	return packageFromJSONResponse(resp, meta)
}

func packageFromJSONResponse(resp *http.Response, meta versioning.DependencyMeta) (pkg Package, err error) {
	if resp.StatusCode != 200 {
		err = errors.Errorf("plugin '%s' does not exist in official repo", meta)
		return
	}
	err = json.NewDecoder(resp.Body).Decode(&pkg)
	if err != nil {
		err = errors.Wrapf(err, "failed to decode plugin package '%s'", meta)
		return
	}
	return
}

func packageFromYAMLResponse(resp *http.Response, meta versioning.DependencyMeta) (pkg Package, err error) {
	if resp.StatusCode != 200 {
		err = errors.Errorf("plugin '%s' does not exist in official repo", meta)
		return
	}
	err = yaml.NewDecoder(resp.Body).Decode(&pkg)
	if err != nil {
		err = errors.Wrapf(err, "failed to decode plugin package '%s'", meta)
		return
	}
	return
}
