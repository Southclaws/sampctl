package pkgcontext

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// Install adds a new dependency to an existing local parent package
func (pcx *PackageContext) Install(
	ctx context.Context,
	targets []versioning.DependencyString,
	development bool,
) (err error) {
	state, err := pcx.beginDependencyOperation(ctx, "install")
	if err != nil {
		return err
	}
	defer func() { err = pcx.finishDependencyOperation(state, err) }()

	changed := false
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		var meta versioning.DependencyMeta
		meta, err = target.Explode()
		if err != nil {
			return &DependencyError{Dependency: string(target), Operation: "parse", Err: err}
		}
		if pcx.declaresDependency(meta) {
			continue
		}
		target, err = pcx.resolveInstallTarget(ctx, target, meta)
		if err != nil {
			return err
		}
		if slices.Contains(pcx.Package.GetAllDependencies(), target) {
			continue
		}
		if development {
			pcx.Package.Development = append(pcx.Package.Development, target)
		} else {
			pcx.Package.Dependencies = append(pcx.Package.Dependencies, target)
		}
		changed = true
	}

	if !changed {
		return nil
	}

	if err := pcx.EnsureDependenciesCachedContext(ctx); err != nil {
		return err
	}

	if err := pcx.EnsureDependencies(ctx, false); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := pcx.writeDependencyDefinition(); err != nil {
		return &OperationError{Operation: "manifest", Err: err}
	}

	return pcx.syncDependencyLockfile(ctx)
}

func (pcx PackageContext) declaresDependency(target versioning.DependencyMeta) bool {
	for _, declaration := range pcx.Package.GetAllDependencies() {
		meta, err := declaration.Explode()
		if err != nil || dependencyUpdateIdentity(meta) != dependencyUpdateIdentity(target) {
			continue
		}
		if !hasExplicitDependencyConstraint(target) || sameDependencyConstraint(meta, target) {
			return true
		}
	}
	return false
}

func (pcx PackageContext) resolveInstallTarget(
	ctx context.Context,
	target versioning.DependencyString,
	meta versioning.DependencyMeta,
) (versioning.DependencyString, error) {
	if meta.IsLocalScheme() || meta.Commit != "" || meta.Branch != "" || meta.Tag != "" {
		return target, nil
	}

	tags, err := pcx.listRepositoryTags(ctx, meta.User, meta.Repo)
	if err != nil {
		return "", &DependencyError{Dependency: string(target), Operation: "resolve tags", Err: err}
	}
	if len(tags) == 0 || tags[0] == nil || tags[0].Name == nil {
		return target, nil
	}

	return versioning.DependencyString(fmt.Sprintf("%s:%s", target, *tags[0].Name)), nil
}

func (pcx PackageContext) listRepositoryTags(
	ctx context.Context,
	user string,
	repo string,
) ([]*github.RepositoryTag, error) {
	client := pcx.GitHub
	if client == nil {
		client = github.NewClient(nil)
	}

	var options github.ListOptions
	tags, _, err := client.Repositories.ListTags(ctx, user, repo, &options)
	if err == nil || !isGitHubBadCredentialsError(err) {
		return tags, err
	}

	print.Verb("GitHub tag lookup failed with credentials, retrying without auth")

	fallbackClient := newAnonymousGitHubClient(client)
	tags, _, fallbackErr := fallbackClient.Repositories.ListTags(ctx, user, repo, &options)
	if fallbackErr != nil {
		return nil, fallbackErr
	}

	return tags, nil
}

func newAnonymousGitHubClient(source *github.Client) *github.Client {
	client := github.NewClient(nil)
	if source == nil {
		return client
	}

	if source.BaseURL != nil {
		baseURL := *source.BaseURL
		client.BaseURL = &baseURL
	}
	if source.UploadURL != nil {
		uploadURL := *source.UploadURL
		client.UploadURL = &uploadURL
	}
	if source.UserAgent != "" {
		client.UserAgent = source.UserAgent
	}

	return client
}

func isGitHubBadCredentialsError(err error) bool {
	var ghErr *github.ErrorResponse
	if !stderrors.As(err, &ghErr) {
		return false
	}
	if ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusUnauthorized {
		return true
	}

	return strings.Contains(strings.ToLower(ghErr.Message), "bad credentials")
}
