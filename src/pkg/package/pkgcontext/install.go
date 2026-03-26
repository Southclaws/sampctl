package pkgcontext

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// Install adds a new dependency to an existing local parent package
func (pcx *PackageContext) Install(
	ctx context.Context,
	targets []versioning.DependencyString,
	development bool,
) (err error) {
	exists := false

	for _, target := range targets {
		var meta versioning.DependencyMeta
		meta, err = target.Explode()
		if err != nil {
			return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
		}

		target, err = pcx.resolveInstallTarget(ctx, target, meta)
		if err != nil {
			return err
		}

		for _, dep := range pcx.Package.GetAllDependencies() {
			if dep == target {
				exists = true
			}
		}

		if !exists {
			if development {
				pcx.Package.Development = append(pcx.Package.Development, target)
			} else {
				pcx.Package.Dependencies = append(pcx.Package.Dependencies, target)
			}
		} else {
			print.Warn("target already exists in dependencies")
			return
		}
	}

	print.Verb(pcx.Package, "ensuring dependencies are cached for package context")
	err = pcx.EnsureDependenciesCached()
	if err != nil {
		return
	}

	print.Verb(pcx.Package, "ensuring dependencies are installed for package context")
	err = pcx.EnsureDependencies(ctx, true)
	if err != nil {
		return
	}

	err = pcx.Package.WriteDefinition()
	if err != nil {
		return
	}

	return nil
}

func (pcx PackageContext) resolveInstallTarget(
	ctx context.Context,
	target versioning.DependencyString,
	meta versioning.DependencyMeta,
) (versioning.DependencyString, error) {
	if meta.Commit != "" || meta.Branch != "" || meta.Tag != "" {
		return target, nil
	}

	tags, err := pcx.listRepositoryTags(ctx, meta.User, meta.Repo)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get repository tags for dependency %s", target)
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

	print.Verb("GitHub tag lookup failed with credentials, retrying without auth:", err)

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
