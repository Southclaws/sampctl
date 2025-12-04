package pawnpackage

import (
	"context"
	"encoding/json"
	"fmt"
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
	data, err := readDefinitionFile(path)
	if err != nil {
		return pkg, err
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

	branch := "master"
	if repo.DefaultBranch != nil && *repo.DefaultBranch != "" {
		branch = *repo.DefaultBranch
	}

	jsonURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/pawn.json",
		meta.User, meta.Repo, branch,
	)
	yamlURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/pawn.yaml",
		meta.User, meta.Repo, branch,
	)

	candidates := []remoteCandidate{
		{url: jsonURL, format: "json"},
		{url: yamlURL, format: "yaml"},
	}

	pkg, err = fetchRemoteDefinition(ctx, candidates)
	if err != nil {
		return pkg, errors.Wrap(err, "package does not point to a valid remote package")
	}

	return pkg, nil
}

// PackageFromOfficialRepo attempts to get a package from the sampctl/plugins official repository
// this repo is mainly only used for testing plugins before being PR'd into their respective repos.
func PackageFromOfficialRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	officialURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json",
		meta.User, meta.Repo,
	)

	candidate := remoteCandidate{
		url:    officialURL,
		format: "json",
		onStatusError: func(status int) error {
			if status == http.StatusNotFound {
				return errors.Errorf("plugin '%s' does not exist in official repo", meta)
			}
			return errors.Errorf("official repository returned %d for '%s'", status, meta)
		},
	}

	return fetchRemoteDefinition(ctx, []remoteCandidate{candidate})
}

type remoteCandidate struct {
	url           string
	format        string
	onStatusError func(status int) error
}

func fetchRemoteDefinition(ctx context.Context, candidates []remoteCandidate) (Package, error) {
	var lastErr error
	for _, candidate := range candidates {
		pkg, err := fetchCandidate(ctx, candidate)
		if err == nil {
			return pkg, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no remote definition candidates provided")
	}
	return Package{}, lastErr
}

func fetchCandidate(ctx context.Context, candidate remoteCandidate) (Package, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.url, nil)
	if err != nil {
		return Package{}, errors.Wrap(err, "failed to build remote definition request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Package{}, errors.Wrap(err, "failed to fetch remote definition")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if candidate.onStatusError != nil {
			return Package{}, candidate.onStatusError(resp.StatusCode)
		}
		return Package{}, errors.Errorf("remote responded with %d for %s", resp.StatusCode, candidate.url)
	}

	var pkg Package
	switch candidate.format {
	case "json":
		err = json.NewDecoder(resp.Body).Decode(&pkg)
	case "yaml":
		err = yaml.NewDecoder(resp.Body).Decode(&pkg)
	default:
		err = errors.Errorf("unsupported remote definition format '%s'", candidate.format)
	}
	if err != nil {
		return Package{}, errors.Wrapf(err, "failed to decode remote definition from %s", candidate.url)
	}

	return pkg, nil
}
