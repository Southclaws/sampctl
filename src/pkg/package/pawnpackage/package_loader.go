package pawnpackage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// PackageFromDir attempts to parse a pawn.json or pawn.yaml file from a directory
func PackageFromDir(dir string) (pkg Package, err error) {
	jsonPath := filepath.Join(dir, "pawn.json")
	yamlPath := filepath.Join(dir, "pawn.yaml")
	jsonExists := fs.Exists(jsonPath)
	yamlExists := fs.Exists(yamlPath)

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
	pkg, err = PackageFromRepo(ctx, client, meta)
	if err != nil {
		print.Verb(meta, "failed to get package definition from repository:", err)
		return PackageFromOfficialRepo(ctx, client, meta)
	}
	return
}

// PackageFromRepo attempts to get a package from the given package definition's public repo
func PackageFromRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	refs := remoteDefinitionRefs(meta)
	paths := []string{"pawn.json", "pawn.yaml"}

	pkg, err = fetchRemoteDefinitionFromGitHub(ctx, client, meta.User, meta.Repo, refs, paths)
	if err != nil {
		return pkg, errors.Wrap(err, "package does not point to a valid remote package")
	}

	return pkg, nil
}

func remoteDefinitionRefs(meta versioning.DependencyMeta) []string {
	refs := make([]string, 0, 3)
	seen := make(map[string]struct{})

	add := func(ref string) {
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	if meta.Tag != "" {
		add(meta.Tag)
	}
	if meta.Branch != "" {
		add(meta.Branch)
	}
	add("") // default branch

	return refs
}

func fetchRemoteDefinitionFromGitHub(
	ctx context.Context,
	client *github.Client,
	owner string,
	repo string,
	refs []string,
	paths []string,
) (Package, error) {
	var lastErr error
	attemptErrors := make([]string, 0, len(refs)*len(paths))

	for _, ref := range refs {
		for _, path := range paths {
			opt := &github.RepositoryContentGetOptions{}
			if ref != "" {
				opt.Ref = ref
			}

			fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, opt)
			if err != nil {
				lastErr = err
				attemptErrors = append(attemptErrors, formatRemoteDefinitionAttemptError(owner, repo, ref, path, err))
				continue
			}
			if fileContent == nil {
				lastErr = errors.Errorf("empty response for %s/%s (%s)", owner, repo, path)
				attemptErrors = append(attemptErrors, formatRemoteDefinitionAttemptError(owner, repo, ref, path, lastErr))
				continue
			}

			rawContent, err := decodeRepositoryContent(fileContent)
			if err != nil {
				lastErr = errors.Wrapf(err, "failed to decode %s/%s (%s)", owner, repo, path)
				attemptErrors = append(attemptErrors, formatRemoteDefinitionAttemptError(owner, repo, ref, path, lastErr))
				continue
			}

			var pkg Package
			switch path {
			case "pawn.json":
				err = json.Unmarshal(rawContent, &pkg)
			case "pawn.yaml":
				err = yaml.Unmarshal(rawContent, &pkg)
			default:
				err = errors.Errorf("unsupported remote definition format for %s", path)
			}
			if err != nil {
				lastErr = errors.Wrapf(err, "failed to parse %s/%s (%s)", owner, repo, path)
				attemptErrors = append(attemptErrors, formatRemoteDefinitionAttemptError(owner, repo, ref, path, lastErr))
				continue
			}

			return pkg, nil
		}
	}

	if len(attemptErrors) > 0 {
		return Package{}, errors.Errorf("failed to load remote package definition after trying %s", strings.Join(attemptErrors, "; "))
	}

	if lastErr == nil {
		lastErr = errors.New("failed to load remote package definition")
	}

	return Package{}, lastErr
}

func formatRemoteDefinitionAttemptError(owner, repo, ref, path string, err error) string {
	location := fmt.Sprintf("%s/%s:%s", owner, repo, path)
	if ref != "" {
		location = fmt.Sprintf("%s@%s", location, ref)
	}
	return fmt.Sprintf("%s (%v)", location, err)
}

func decodeRepositoryContent(fileContent *github.RepositoryContent) ([]byte, error) {
	if fileContent == nil {
		return nil, errors.New("nil file content")
	}

	content, err := fileContent.GetContent()
	if err == nil && content != "" {
		return []byte(content), nil
	}

	if fileContent.Content == nil || *fileContent.Content == "" {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("missing repository content")
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(*fileContent.Content, "\n", ""))
	if err != nil {
		return nil, err
	}

	return decoded, nil
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
