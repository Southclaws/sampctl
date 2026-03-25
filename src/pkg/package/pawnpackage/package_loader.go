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

var (
	packageDefinitionHTTPClient = http.DefaultClient
	officialPackageRepoBaseURL  = "https://raw.githubusercontent.com/sampctl/plugins/master"
)

// GitHubRemotePackageFetcher fetches package definitions from GitHub and the
// official fallback repository.
type GitHubRemotePackageFetcher struct {
	GitHub          *github.Client
	HTTPClient      *http.Client
	OfficialBaseURL string
}

// NewRemotePackageFetcher builds the default remote package fetcher.
func NewRemotePackageFetcher(client *github.Client) RemotePackageFetcher {
	return &GitHubRemotePackageFetcher{
		GitHub:          client,
		HTTPClient:      packageDefinitionHTTPClient,
		OfficialBaseURL: officialPackageRepoBaseURL,
	}
}

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
	return NewRemotePackageFetcher(client).Fetch(ctx, meta)
}

// PackageFromRepo attempts to get a package from the given package definition's public repo
func PackageFromRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	return (&GitHubRemotePackageFetcher{GitHub: client}).packageFromRepo(ctx, meta)
}

func (f *GitHubRemotePackageFetcher) Fetch(ctx context.Context, meta versioning.DependencyMeta) (pkg Package, err error) {
	pkg, err = f.packageFromRepo(ctx, meta)
	if err != nil {
		print.Verb(meta, "failed to get package definition from repository:", err)
		return f.packageFromOfficialRepo(ctx, meta)
	}
	return pkg, nil
}

func (f *GitHubRemotePackageFetcher) packageFromRepo(
	ctx context.Context,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	refs := remoteDefinitionRefs(meta)
	paths := []string{"pawn.json", "pawn.yaml"}

	pkg, err = fetchRemoteDefinitionFromGitHub(remoteDefinitionRequest{
		Context: ctx,
		Client:  f.GitHub,
		Owner:   meta.User,
		Repo:    meta.Repo,
		Refs:    refs,
		Paths:   paths,
	})
	if err != nil {
		return pkg, errors.Wrap(err, "package does not point to a valid remote package")
	}

	return pkg, nil
}

func remoteDefinitionRefs(meta versioning.DependencyMeta) []string {
	refs := make([]string, 0, 4)
	seen := make(map[string]struct{})

	add := func(ref string) {
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	if meta.Commit != "" {
		add(meta.Commit)
	}
	if meta.Branch != "" {
		add(meta.Branch)
	}
	if meta.Tag != "" {
		add(meta.Tag)
	}
	add("") // default branch

	return refs
}

type remoteDefinitionRequest struct {
	Context context.Context
	Client  *github.Client
	Owner   string
	Repo    string
	Refs    []string
	Paths   []string
}

func fetchRemoteDefinitionFromGitHub(request remoteDefinitionRequest) (Package, error) {
	if request.Client == nil {
		return Package{}, errors.New("no GitHub client provided")
	}

	var lastErr error

	for _, ref := range request.Refs {
		for _, path := range request.Paths {
			opt := &github.RepositoryContentGetOptions{}
			if ref != "" {
				opt.Ref = ref
			}

			fileContent, _, _, err := request.Client.Repositories.GetContents(request.Context, request.Owner, request.Repo, path, opt)
			if err != nil {
				lastErr = err
				continue
			}
			if fileContent == nil {
				lastErr = errors.Errorf("empty response for %s/%s (%s)", request.Owner, request.Repo, path)
				continue
			}

			rawContent, err := decodeRepositoryContent(fileContent)
			if err != nil {
				lastErr = errors.Wrapf(err, "failed to decode %s/%s (%s)", request.Owner, request.Repo, path)
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
				lastErr = errors.Wrapf(err, "failed to parse %s/%s (%s)", request.Owner, request.Repo, path)
				continue
			}

			return pkg, nil
		}
	}

	if lastErr == nil {
		lastErr = errors.New("failed to load remote package definition")
	}

	return Package{}, lastErr
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
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	return (&GitHubRemotePackageFetcher{HTTPClient: packageDefinitionHTTPClient, OfficialBaseURL: officialPackageRepoBaseURL}).packageFromOfficialRepo(ctx, meta)
}

func (f *GitHubRemotePackageFetcher) packageFromOfficialRepo(
	ctx context.Context,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	baseURL := f.OfficialBaseURL
	if baseURL == "" {
		baseURL = officialPackageRepoBaseURL
	}
	client := f.HTTPClient
	if client == nil {
		client = packageDefinitionHTTPClient
	}

	officialURL := fmt.Sprintf(
		"%s/%s-%s.json",
		strings.TrimRight(baseURL, "/"),
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

	return fetchRemoteDefinition(ctx, client, []remoteCandidate{candidate})
}

type remoteCandidate struct {
	url           string
	format        string
	onStatusError func(status int) error
}

func fetchRemoteDefinition(ctx context.Context, client *http.Client, candidates []remoteCandidate) (Package, error) {
	var lastErr error
	for _, candidate := range candidates {
		pkg, err := fetchCandidate(ctx, client, candidate)
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

func fetchCandidate(ctx context.Context, client *http.Client, candidate remoteCandidate) (Package, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.url, nil)
	if err != nil {
		return Package{}, errors.Wrap(err, "failed to build remote definition request")
	}

	if client == nil {
		client = packageDefinitionHTTPClient
	}

	resp, err := client.Do(req)
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
